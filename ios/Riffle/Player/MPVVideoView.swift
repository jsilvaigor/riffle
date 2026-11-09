import SwiftUI
import UIKit

/// Hosts the Metal layer mpv draws into. mpv handles aspect, letterboxing (in
/// `stage` black) and fit/fill itself, so the layer simply fills the view.
struct MPVVideoView: UIViewRepresentable {
    let layer: MPVMetalLayer
    /// Called once the layer is in a window with a real size. mpv must not create
    /// its swapchain before then, or it can render into a surface that never shows.
    var onReady: () -> Void
    /// Called when the layer's size changes after it was ready (rotation).
    var onResize: () -> Void = {}

    func makeUIView(context: Context) -> LayerHostView {
        let view = LayerHostView()
        view.onReady = onReady
        view.onResize = onResize
        view.backgroundColor = .black
        layer.backgroundColor = UIColor.black.cgColor
        layer.framebufferOnly = true
        view.layer.addSublayer(layer)
        return view
    }

    func updateUIView(_ view: LayerHostView, context: Context) {}

    final class LayerHostView: UIView {
        var onReady: (() -> Void)?
        var onResize: () -> Void = {}
        private var lastSize: CGSize = .zero
        private var resizeTask: Task<Void, Never>?

        override func layoutSubviews() {
            super.layoutSubviews()
            guard let metal = layer.sublayers?.first as? CAMetalLayer else { return }
            let scale = window?.screen.nativeScale ?? traitCollection.displayScale
            let target = CGSize(width: bounds.width * scale, height: bounds.height * scale)
            CATransaction.begin()
            CATransaction.setDisableActions(true)
            metal.frame = bounds
            metal.contentsScale = scale
            CATransaction.commit()
            guard window != nil, bounds.width > 1, bounds.height > 1 else { return }

            if let ready = onReady {
                // Before mpv exists nothing renders, so the size can be set directly.
                metal.drawableSize = target
                onReady = nil
                lastSize = bounds.size
                ready()
            } else if bounds.size != lastSize {
                lastSize = bounds.size
                resize(metal, to: target)
            }
        }

        /// A size change while mpv renders (rotation). Setting `drawableSize` here would
        /// hand out drawables of the new size while mpv's Vulkan swapchain still renders
        /// the old one: a mismatched frame, which Metal API validation stops with
        /// "renderTargetHeight must be <= minimum attachment height". Instead, let
        /// MoltenVK notice the new bounds and rebuild the swapchain on mpv's next frame
        /// (it sets `drawableSize` itself, consistently), and only then tell mpv to
        /// reconfigure so it lays the picture out for the new size. If nothing renders
        /// for a while (paused), set the size directly: with no frames in flight there's
        /// nothing to mismatch.
        private func resize(_ metal: CAMetalLayer, to target: CGSize) {
            resizeTask?.cancel()
            resizeTask = Task { @MainActor [weak self] in
                for _ in 0..<25 where metal.drawableSize != target {
                    try? await Task.sleep(for: .milliseconds(20))
                    if Task.isCancelled { return }
                }
                if metal.drawableSize != target { metal.drawableSize = target }
                self?.onResize()
            }
        }
    }
}

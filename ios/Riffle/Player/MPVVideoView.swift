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

        override func layoutSubviews() {
            super.layoutSubviews()
            guard let metal = layer.sublayers?.first as? CAMetalLayer else { return }
            CATransaction.begin()
            CATransaction.setDisableActions(true)
            let scale = window?.screen.nativeScale ?? traitCollection.displayScale
            metal.frame = bounds
            metal.contentsScale = scale
            // Once MoltenVK has set drawableSize it stops following the bounds, so a
            // rotation would keep the old size; mpv rebuilds its swapchain on change.
            metal.drawableSize = CGSize(width: bounds.width * scale, height: bounds.height * scale)
            CATransaction.commit()
            guard window != nil, bounds.width > 1, bounds.height > 1 else { return }
            if let ready = onReady {
                onReady = nil
                lastSize = bounds.size
                ready()
            } else if bounds.size != lastSize {
                lastSize = bounds.size
                onResize()
            }
        }
    }
}

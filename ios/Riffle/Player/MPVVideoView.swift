import SwiftUI
import UIKit

/// Hosts the Metal layer mpv draws into. mpv handles aspect, letterboxing (in
/// `stage` black) and fit/fill itself, so the layer simply fills the view.
struct MPVVideoView: UIViewRepresentable {
    let layer: MPVMetalLayer

    func makeUIView(context: Context) -> LayerHostView {
        let view = LayerHostView()
        view.backgroundColor = .black
        layer.backgroundColor = UIColor.black.cgColor
        layer.framebufferOnly = true
        view.layer.addSublayer(layer)
        return view
    }

    func updateUIView(_ view: LayerHostView, context: Context) {}

    final class LayerHostView: UIView {
        override func layoutSubviews() {
            super.layoutSubviews()
            guard let metal = layer.sublayers?.first as? CAMetalLayer else { return }
            CATransaction.begin()
            CATransaction.setDisableActions(true)
            metal.frame = bounds
            metal.contentsScale = window?.screen.nativeScale ?? traitCollection.displayScale
            CATransaction.commit()
            #if DEBUG
            print("[video] layer frame \(metal.frame) scale \(metal.contentsScale) drawable \(metal.drawableSize) window \(window != nil)")
            #endif
        }
    }
}

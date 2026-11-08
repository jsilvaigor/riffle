import SwiftUI
import UIKit

/// The video's gestures (player.md, Gestures), in UIKit so taps, double taps, pans,
/// holds and pinches can be told apart precisely. Sits under the controls: gestures
/// that start on a control belong to the control.
struct GestureLayer: UIViewRepresentable {
    struct Handlers {
        var tap: () -> Void
        /// Side, and whether this tap continues a skip run.
        var skip: (_ forward: Bool, _ continuing: Bool) -> Void
        var scrubBegan: () -> Void
        /// Horizontal translation as a fraction of the view's width.
        var scrubChanged: (_ fraction: CGFloat) -> Void
        var scrubEnded: () -> Void
        var dismissChanged: (_ dy: CGFloat) -> Void
        var dismissEnded: (_ dy: CGFloat, _ velocity: CGFloat) -> Void
        var hold: (_ active: Bool) -> Void
        var pinch: (_ fill: Bool) -> Void
    }

    var handlers: Handlers
    var dismissEnabled: Bool

    func makeCoordinator() -> Coordinator { Coordinator(handlers: handlers) }

    func makeUIView(context: Context) -> UIView {
        let view = UIView()
        view.backgroundColor = .clear
        let c = context.coordinator
        c.view = view

        let tap = UITapGestureRecognizer(target: c, action: #selector(Coordinator.onTap(_:)))
        let pan = UIPanGestureRecognizer(target: c, action: #selector(Coordinator.onPan(_:)))
        pan.maximumNumberOfTouches = 1
        let hold = UILongPressGestureRecognizer(target: c, action: #selector(Coordinator.onHold(_:)))
        hold.minimumPressDuration = 0.5
        hold.delegate = c
        let pinch = UIPinchGestureRecognizer(target: c, action: #selector(Coordinator.onPinch(_:)))
        [tap, pan, hold, pinch].forEach(view.addGestureRecognizer)
        return view
    }

    func updateUIView(_ view: UIView, context: Context) {
        context.coordinator.handlers = handlers
        context.coordinator.dismissEnabled = dismissEnabled
    }

    @MainActor
    final class Coordinator: NSObject, UIGestureRecognizerDelegate {
        var handlers: Handlers
        var dismissEnabled = true
        weak var view: UIView?

        private enum PanMode { case scrub, dismiss, none }
        private var panMode = PanMode.none
        private var pendingTap: DispatchWorkItem?
        private var pendingSide: Bool?
        private var lastSkip: (forward: Bool, at: Date)?

        init(handlers: Handlers) { self.handlers = handlers }

        /// One tap toggles chrome; a second tap on a side third within 0.3 s skips
        /// 10 s, and each further tap within 0.6 s adds 10 s. Taps in the middle
        /// third toggle at once, without waiting for a double tap.
        @objc func onTap(_ g: UITapGestureRecognizer) {
            guard let view else { return }
            let x = g.location(in: view).x / max(view.bounds.width, 1)
            let side: Bool? = x < 1 / 3 ? false : (x > 2 / 3 ? true : nil)

            if let side, let last = lastSkip, last.forward == side, Date.now.timeIntervalSince(last.at) < 0.6 {
                lastSkip = (side, .now)
                handlers.skip(side, true)
                return
            }
            if let side, let pending = pendingTap, pendingSide == side {
                pending.cancel()
                pendingTap = nil
                lastSkip = (side, .now)
                handlers.skip(side, false)
                return
            }
            pendingTap?.cancel()
            guard let side else {
                pendingTap = nil
                handlers.tap()
                return
            }
            let work = DispatchWorkItem { [weak self] in
                self?.pendingTap = nil
                self?.handlers.tap()
            }
            pendingTap = work
            pendingSide = side
            DispatchQueue.main.asyncAfter(deadline: .now() + 0.3, execute: work)
        }

        @objc func onPan(_ g: UIPanGestureRecognizer) {
            guard let view else { return }
            let t = g.translation(in: view)
            switch g.state {
            case .began:
                let v = g.velocity(in: view)
                if abs(v.x) > abs(v.y) {
                    panMode = .scrub
                    handlers.scrubBegan()
                } else if v.y > 0 && dismissEnabled {
                    panMode = .dismiss
                } else {
                    panMode = .none
                }
            case .changed:
                switch panMode {
                case .scrub: handlers.scrubChanged(t.x / max(view.bounds.width, 1))
                case .dismiss: handlers.dismissChanged(max(0, t.y))
                case .none: break
                }
            case .ended, .cancelled, .failed:
                switch panMode {
                case .scrub: handlers.scrubEnded()
                case .dismiss: handlers.dismissEnded(max(0, t.y), g.velocity(in: view).y)
                case .none: break
                }
                panMode = .none
            default: break
            }
        }

        @objc func onHold(_ g: UILongPressGestureRecognizer) {
            switch g.state {
            case .began: handlers.hold(true)
            case .ended, .cancelled, .failed: handlers.hold(false)
            default: break
            }
        }

        @objc func onPinch(_ g: UIPinchGestureRecognizer) {
            guard g.state == .ended else { return }
            if g.scale > 1.1 { handlers.pinch(true) } else if g.scale < 0.9 { handlers.pinch(false) }
        }

        /// Hold only on the right half.
        func gestureRecognizerShouldBegin(_ g: UIGestureRecognizer) -> Bool {
            guard g is UILongPressGestureRecognizer, let view else { return true }
            return g.location(in: view).x > view.bounds.width / 2
        }
    }
}

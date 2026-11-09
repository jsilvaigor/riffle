import SwiftUI
import UIKit

/// Presents the player full screen from UIKit, in a hosting controller that answers
/// for its own orientations. SwiftUI's `fullScreenCover` controller keeps reporting
/// the library's portrait-only mask, so iOS refuses the landscape request for wide
/// videos (player.md, Orientation).
@MainActor
enum PlayerPresenter {
    static func present(_ request: PlaybackRequest, model: AppModel) {
        guard let top = topViewController(), !(top is PlayerHostingController) else { return }
        let host = PlayerHostingController(rootView: AnyView(EmptyView()))
        host.rootView = AnyView(
            PlayerView(
                request: request,
                onClose: { [weak host] in host?.dismiss(animated: true) },
                setOverlaysHidden: { [weak host] in host?.overlaysHidden = $0 }
            )
            .environment(model)
        )
        host.modalPresentationStyle = .fullScreen
        host.view.backgroundColor = .black
        Orientation.preparePlayer()
        top.present(host, animated: true) { Orientation.playerDidAppear() }
    }

    private static func topViewController() -> UIViewController? {
        let scene = UIApplication.shared.connectedScenes.compactMap { $0 as? UIWindowScene }
            .first { $0.activationState == .foregroundActive }
        var vc = scene?.windows.first { $0.isKeyWindow }?.rootViewController ?? scene?.windows.first?.rootViewController
        while let presented = vc?.presentedViewController { vc = presented }
        return vc
    }
}

final class PlayerHostingController: UIHostingController<AnyView> {
    /// Status bar and home indicator hide with the chrome.
    var overlaysHidden = false {
        didSet {
            guard overlaysHidden != oldValue else { return }
            setNeedsStatusBarAppearanceUpdate()
            setNeedsUpdateOfHomeIndicatorAutoHidden()
        }
    }

    override var supportedInterfaceOrientations: UIInterfaceOrientationMask {
        Orientation.mask
    }
    override var prefersStatusBarHidden: Bool { overlaysHidden }
    override var prefersHomeIndicatorAutoHidden: Bool { overlaysHidden }
    override var preferredStatusBarStyle: UIStatusBarStyle { .lightContent }
}

import UIKit

/// What the player allows (player.md, Orientation). The library's controller is
/// always portrait on iPhone; the player's controller answers with `mask`, which
/// follows the video on open, then the phone, unless rotation lock pins it.
@MainActor
enum Orientation {
    private static var isPad: Bool { UIDevice.current.userInterfaceIdiom == .pad }

    static var libraryMask: UIInterfaceOrientationMask { isPad ? .all : .portrait }
    private static var playerMask: UIInterfaceOrientationMask { isPad ? .all : .allButUpsideDown }

    /// The player's allowed orientations.
    private(set) static var mask: UIInterfaceOrientationMask = .allButUpsideDown

    // While the player is still presenting, the library governs orientation and a
    // request would be refused, so a landscape request waits until it's on screen.
    private static var playerOnScreen = false
    private static var pendingLandscape = false

    static func preparePlayer() {
        mask = playerMask
        playerOnScreen = false
        pendingLandscape = false
    }

    static func playerDidAppear() {
        playerOnScreen = true
        apply()
        if pendingLandscape {
            pendingLandscape = false
            request(.landscapeRight)
        }
    }

    /// Wide videos turn to landscape once, on open, and only on iPhone.
    static func enterPlayer(requestLandscape: Bool) {
        guard requestLandscape && !isPad else { return }
        if playerOnScreen { request(.landscapeRight) } else { pendingLandscape = true }
    }

    static var canRotate: Bool { !isPad }

    /// Whether the interface is landscape right now.
    static var isLandscape: Bool {
        scene?.effectiveGeometry.interfaceOrientation.isLandscape ?? false
    }

    /// The Rotate button: turn between portrait and landscape for this video. A
    /// geometry request turns the interface even with the system's portrait lock on,
    /// and with that lock nothing turns it back until Rotate is tapped again.
    /// With Rotation lock on, the lock moves to the new orientation.
    static func toggleRotation(locked: Bool) {
        guard canRotate else { return }
        let target: UIInterfaceOrientationMask = isLandscape ? .portrait : .landscapeRight
        if locked {
            mask = target
            apply()
        }
        request(target)
    }

    /// Rotation lock: pin whatever orientation the interface is in now.
    static func lockCurrent() {
        guard let scene else { return }
        mask = switch scene.effectiveGeometry.interfaceOrientation {
        case .landscapeLeft: .landscapeLeft
        case .landscapeRight: .landscapeRight
        case .portraitUpsideDown: .portraitUpsideDown
        default: .portrait
        }
        apply()
    }

    static func unlock() {
        mask = playerMask
        apply()
    }

    /// The library's controller takes over again on dismissal and turns back to
    /// portrait by itself.
    static func exitPlayer() {
        playerOnScreen = false
        pendingLandscape = false
        mask = playerMask
    }

    /// Turns the interface without changing what's allowed afterwards. A refused
    /// request (UIKit can lag a moment behind a mask change) is retried once.
    static func request(_ orientations: UIInterfaceOrientationMask, retry: Bool = true) {
        scene?.requestGeometryUpdate(.iOS(interfaceOrientations: orientations)) { error in
            #if DEBUG
            print("[orientation] request \(orientations.rawValue) failed: \(error.localizedDescription)")
            #endif
            guard retry else { return }
            Task { @MainActor in
                try? await Task.sleep(for: .milliseconds(300))
                guard playerOnScreen, mask.contains(orientations) else { return }
                request(orientations, retry: false)
            }
        }
    }

    private static var scene: UIWindowScene? {
        UIApplication.shared.connectedScenes.compactMap { $0 as? UIWindowScene }
            .first { $0.activationState == .foregroundActive } ??
            UIApplication.shared.connectedScenes.compactMap { $0 as? UIWindowScene }.first
    }

    /// Asks the controllers to re-read their supported orientations.
    private static func apply() {
        for window in scene?.windows ?? [] {
            var vc = window.rootViewController
            while let current = vc {
                current.setNeedsUpdateOfSupportedInterfaceOrientations()
                vc = current.presentedViewController
            }
        }
    }
}

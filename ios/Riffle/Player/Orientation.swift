import UIKit

/// Which orientations the app allows right now. The library is portrait on iPhone;
/// the player follows the video and then the phone (player.md, Orientation).
@MainActor
enum Orientation {
    private static var isPad: Bool { UIDevice.current.userInterfaceIdiom == .pad }

    static var libraryMask: UIInterfaceOrientationMask { isPad ? .all : .portrait }
    private(set) static var mask: UIInterfaceOrientationMask = libraryMask

    /// Opens the player: allow every orientation, and on iPhone turn to landscape
    /// once if the video is wide and the setting allows it.
    static func enterPlayer(requestLandscape: Bool) {
        mask = isPad ? .all : .allButUpsideDown
        apply()
        if requestLandscape && !isPad {
            // UIKit re-reads the app delegate's mask asynchronously after `apply()`;
            // a request sent before that is checked against the old portrait mask.
            Task {
                try? await Task.sleep(for: .milliseconds(250))
                request(.landscapeRight)
            }
        }
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
        mask = isPad ? .all : .allButUpsideDown
        apply()
    }

    static func exitPlayer() {
        mask = libraryMask
        apply()
        if !isPad { request(.portrait) }
    }

    /// Turns the interface without changing what's allowed afterwards.
    static func request(_ orientations: UIInterfaceOrientationMask) {
        scene?.requestGeometryUpdate(.iOS(interfaceOrientations: orientations)) { error in
            #if DEBUG
            print("[orientation] request \(orientations.rawValue) failed: \(error.localizedDescription)")
            #endif
        }
    }

    private static var scene: UIWindowScene? {
        UIApplication.shared.connectedScenes.compactMap { $0 as? UIWindowScene }
            .first { $0.activationState == .foregroundActive } ??
            UIApplication.shared.connectedScenes.compactMap { $0 as? UIWindowScene }.first
    }

    /// Asks every controller in the scene to re-read the allowed orientations (which
    /// makes UIKit ask the app delegate again).
    private static func apply() {
        var updated = 0
        for window in scene?.windows ?? [] {
            var vc = window.rootViewController
            while let current = vc {
                current.setNeedsUpdateOfSupportedInterfaceOrientations()
                updated += 1
                vc = current.presentedViewController
            }
        }
        #if DEBUG
        print("[orientation] mask \(mask.rawValue) applied to \(updated) controllers")
        #endif
    }
}

final class AppDelegate: NSObject, UIApplicationDelegate {
    func application(_ application: UIApplication,
                     supportedInterfaceOrientationsFor window: UIWindow?) -> UIInterfaceOrientationMask {
        MainActor.assumeIsolated {
            #if DEBUG
            print("[orientation] app delegate asked, mask \(Orientation.mask.rawValue)")
            #endif
            return Orientation.mask
        }
    }
}

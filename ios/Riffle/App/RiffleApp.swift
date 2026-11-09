import SwiftUI
import UIKit

// The app starts from UIKit so the window's root controller is ours: SwiftUI's own
// root caches the allowed orientations at launch, which made the player's landscape
// request for wide videos fail intermittently. The screens are all SwiftUI.

@main
final class AppDelegate: UIResponder, UIApplicationDelegate {}

/// Named in Info.plist's scene manifest.

final class SceneDelegate: UIResponder, UIWindowSceneDelegate {
    var window: UIWindow?
    private let model = AppModel()

    func scene(_ scene: UIScene, willConnectTo session: UISceneSession,
               options connectionOptions: UIScene.ConnectionOptions) {
        guard let windowScene = scene as? UIWindowScene else { return }
        let window = UIWindow(windowScene: windowScene)
        window.overrideUserInterfaceStyle = .dark
        window.tintColor = UIColor(Riffle.Palette.citron)
        window.rootViewController = RootHostingController(rootView: AnyView(
            RootView()
                .environment(model)
                .preferredColorScheme(.dark)
                .tint(Riffle.Palette.citron)
        ))
        window.makeKeyAndVisible()
        self.window = window
    }
}

/// The library: portrait on iPhone, any way on iPad. The player is presented over it
/// in its own controller, which decides while it's up.
final class RootHostingController: UIHostingController<AnyView> {
    override var supportedInterfaceOrientations: UIInterfaceOrientationMask { Orientation.libraryMask }
}

struct RootView: View {
    @Environment(AppModel.self) private var model

    var body: some View {
        Group {
            if model.isConfigured {
                MainTabs()
            } else {
                ConnectView()
            }
        }
        .background(Riffle.Palette.surface)
        .task(id: model.serverAddress) { await model.refresh() }
        .onReceive(NotificationCenter.default.publisher(for: UIApplication.willEnterForegroundNotification)) { _ in
            Task { await model.refresh() }
        }
    }
}

struct MainTabs: View {
    var body: some View {
        TabView {
            Tab("Library", systemImage: "rectangle.stack") { LibraryView() }
            Tab("Search", systemImage: "magnifyingglass", role: .search) { SearchView() }
            Tab("Settings", systemImage: "slider.horizontal.3") { SettingsView() }
        }
    }
}

/// What the player is asked to open. The detail screen decides where to start, so the
/// player never prompts (design/player.md, Resume).
struct PlaybackRequest: Identifiable, Hashable {
    var item: Item
    var start: Double
    var id: String { item.id }
}

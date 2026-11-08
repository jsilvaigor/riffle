import SwiftUI

@main
struct RiffleApp: App {
    @UIApplicationDelegateAdaptor(AppDelegate.self) private var appDelegate
    @State private var model = AppModel()

    var body: some Scene {
        WindowGroup {
            RootView()
                .environment(model)
                .preferredColorScheme(.dark)
                .tint(Riffle.Palette.citron)
        }
    }
}

struct RootView: View {
    @Environment(AppModel.self) private var model
    @Environment(\.scenePhase) private var scenePhase

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
        .onChange(of: scenePhase) { _, phase in
            if phase == .active { Task { await model.refresh() } }
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

import SwiftUI

/// The Library tab: server status when it matters, Continue watching, then the folders.
struct LibraryView: View {
    @Environment(AppModel.self) private var model
    @State private var playing: PlaybackRequest?
    @State private var debugAutoplayed = false

    var body: some View {
        NavigationStack {
            content
                .navigationTitle("Library")
                .riffleScreen()
                .refreshable { await model.refresh() }
                .libraryDestinations(playing: $playing)
        }
        .playerCover($playing)
        #if DEBUG
        // `defaults write dev.isilva.riffle debug.autoplay <item id>` opens it at launch.
        .task(id: model.library?.items.count) {
            guard !debugAutoplayed, let id = UserDefaults.standard.string(forKey: "debug.autoplay"),
                  let item = model.item(id) else { return }
            debugAutoplayed = true
            try? await Task.sleep(for: .seconds(1))  // let the window finish appearing
            playing = PlaybackRequest(item: item, start: 0)
        }
        #endif
    }

    @ViewBuilder private var content: some View {
        if model.library == nil {
            switch model.connection {
            case .offline(let message):
                EmptyStateView(illustration: .offline, title: "Can't reach Riffle Server", message: message) {
                    Button("Try again") { Task { await model.refresh() } }.buttonStyle(.rifflePlain)
                }
                .frame(maxWidth: .infinity, maxHeight: .infinity)
            default:
                ProgressView().tint(Riffle.Palette.ink).frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        } else if model.library?.items.isEmpty == true {
            ScrollView {
                EmptyStateView(
                    illustration: .emptyLibrary,
                    title: "No videos yet",
                    message: "Add folders to RIFFLE_MEDIA on the server, then rescan."
                ) {
                    Button("Rescan library") { Task { try? await model.rescan() } }.buttonStyle(.rifflePlain)
                }
                .padding(.top, 80)
                .frame(maxWidth: .infinity)
            }
        } else {
            List {
                if case .connected = model.connection {} else {
                    ServerStatusView(state: model.connection, address: model.serverAddress)
                        .riffleRow()
                }
                if !model.continueWatching.isEmpty {
                    Section {
                        ContinueWatchingShelf(items: model.continueWatching)
                            .listRowInsets(EdgeInsets())
                            .riffleRow(separator: false)
                    } header: {
                        SectionHeader("Continue watching")
                    }
                }
                Section {
                    // With a single root, skip a level and show its contents directly.
                    if model.roots.count == 1, let root = model.roots.first {
                        FolderContents(folder: root)
                    } else {
                        ForEach(model.roots) { root in
                            NavigationLink(value: root) { FolderRowView(folder: root, counts: model.counts(root)) }.riffleRow()
                        }
                    }
                } header: {
                    if model.roots.count == 1, let root = model.roots.first {
                        SectionHeader(root.name, caption: model.counts(root).summary)
                    } else {
                        SectionHeader("Folders")
                    }
                }
            }
            .listStyle(.plain)
        }
    }
}

struct ContinueWatchingShelf: View {
    @Environment(AppModel.self) private var model
    var items: [Item]
    @Environment(\.horizontalSizeClass) private var sizeClass

    var body: some View {
        ScrollView(.horizontal, showsIndicators: false) {
            LazyHStack(alignment: .top, spacing: sizeClass == .regular ? Riffle.Spacing.s4 : Riffle.Spacing.s2 + 4) {
                ForEach(items) { item in
                    NavigationLink(value: item) {
                        VideoCardView(item: item, meta: remaining(item), progress: model.progress[item.id])
                            .frame(width: sizeClass == .regular ? 300 : 240)
                    }
                    .buttonStyle(.plain)
                    .watchedMenu(item.id)
                }
            }
            .padding(.horizontal, Riffle.Spacing.s4)
            .padding(.bottom, Riffle.Spacing.s4)
        }
    }

    private func remaining(_ item: Item) -> String {
        guard let p = model.progress[item.id] else { return item.root }
        return "\(Format.time(max(p.duration - p.position, 0))) left · \(item.root)"
    }
}

/// The rows of a folder: subfolders first, then videos.
struct FolderContents: View {
    @Environment(AppModel.self) private var model
    var folder: Folder

    var body: some View {
        ForEach(folder.folders) { sub in
            NavigationLink(value: sub) { FolderRowView(folder: sub, counts: model.counts(sub)) }.riffleRow()
        }
        ForEach(folder.videos) { item in
            NavigationLink(value: item) { VideoRowView(item: item, progress: model.progress[item.id]) }
                .watchedMenu(item.id)
                .riffleRow()
        }
    }
}

struct FolderView: View {
    var folder: Folder

    var body: some View {
        List { FolderContents(folder: folder) }
            .listStyle(.plain)
            .navigationTitle(folder.name)
            .navigationBarTitleDisplayMode(.inline)
            .riffleScreen()
    }
}

struct SectionHeader: View {
    var title: String
    var caption: String?
    init(_ title: String, caption: String? = nil) {
        self.title = title
        self.caption = caption
    }

    var body: some View {
        VStack(alignment: .leading, spacing: Riffle.Spacing.s1) {
            Text(title)
                .font(Riffle.Typography.title)
                .foregroundStyle(Riffle.Palette.ink)
            if let caption {
                Text(caption)
                    .font(Riffle.Typography.caption)
                    .foregroundStyle(Riffle.Palette.inkMuted)
            }
        }
        .textCase(nil)
        .padding(.top, Riffle.Spacing.s2)
    }
}

/// Long press on a video: mark it watched or unwatched.
private struct WatchedMenu: ViewModifier {
    @Environment(AppModel.self) private var model
    var id: String

    func body(content: Content) -> some View {
        content.contextMenu {
            if model.state(id) == .watched {
                Button("Mark as unwatched", systemImage: "circle") { Task { await model.setWatched(id, false) } }
            } else {
                Button("Mark as watched", systemImage: "checkmark.circle") { Task { await model.setWatched(id, true) } }
            }
        }
    }
}

extension View {
    func watchedMenu(_ id: String) -> some View { modifier(WatchedMenu(id: id)) }
}

extension View {
    /// `surface` behind a whole screen, including under lists.
    func riffleScreen() -> some View {
        self
            .scrollContentBackground(.hidden)
            .background(Riffle.Palette.surface)
            .toolbarBackground(Riffle.Palette.surface, for: .navigationBar)
    }

    func riffleRow(separator: Bool = true) -> some View {
        self
            .listRowBackground(Riffle.Palette.surface)
            .listRowSeparatorTint(Riffle.Palette.line)
            .listRowSeparator(separator ? .automatic : .hidden)
    }

    /// Folder and video destinations shared by the Library and Search stacks.
    func libraryDestinations(playing: Binding<PlaybackRequest?>) -> some View {
        self
            .navigationDestination(for: Folder.self) { FolderView(folder: $0) }
            .navigationDestination(for: Item.self) { DetailView(itemID: $0.id, fallback: $0, playing: playing) }
    }

    func playerCover(_ request: Binding<PlaybackRequest?>) -> some View {
        modifier(PlayerCover(request: request))
    }
}

/// Hands a playback request to `PlayerPresenter`, then clears it.
private struct PlayerCover: ViewModifier {
    @Environment(AppModel.self) private var model
    @Binding var request: PlaybackRequest?

    func body(content: Content) -> some View {
        content.onChange(of: request) { _, new in
            guard let new else { return }
            PlayerPresenter.present(new, model: model)
            request = nil
        }
    }
}

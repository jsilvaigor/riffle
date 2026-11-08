import SwiftUI

/// Search by file or folder name across the whole library.
struct SearchView: View {
    @Environment(AppModel.self) private var model
    @State private var query = ""
    @State private var playing: PlaybackRequest?

    private var results: [Item] {
        let q = query.trimmingCharacters(in: .whitespaces)
        guard !q.isEmpty, let items = model.library?.items else { return [] }
        return items
            .filter { $0.path.localizedStandardContains(q) || $0.root.localizedStandardContains(q) }
            .sorted { $0.name.localizedStandardCompare($1.name) == .orderedAscending }
    }

    var body: some View {
        NavigationStack {
            List {
                ForEach(results) { item in
                    NavigationLink(value: item) {
                        VideoRowView(item: item, progress: model.progress[item.id])
                    }
                    .riffleRow()
                }
            }
            .listStyle(.plain)
            .overlay {
                if !query.isEmpty && results.isEmpty {
                    ContentUnavailableView.search(text: query)
                }
            }
            .navigationTitle("Search")
            .searchable(text: $query, prompt: "Videos and folders")
            .riffleScreen()
            .libraryDestinations(playing: $playing)
        }
        .playerCover($playing)
    }
}

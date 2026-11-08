import Foundation

/// The library as folders: roots at the top, then the folders under each, with videos
/// at every level. Built from the flat item list the server returns.
struct Folder: Identifiable, Hashable, Sendable {
    /// "Movies" or "Movies/Pixar/Shorts".
    var id: String
    var name: String
    var folders: [Folder]
    var videos: [Item]
    /// Videos in this folder and every folder below it.
    var count: Int

    static func roots(from library: LibraryResponse) -> [Folder] {
        let byRoot = Dictionary(grouping: library.items, by: \.root)
        // Keep the server's root order; add any root only items mention.
        var names = library.roots
        for root in byRoot.keys.sorted() where !names.contains(root) { names.append(root) }
        return names.map { build(id: $0, name: $0, items: byRoot[$0] ?? [], depth: 0) }
    }

    private static func build(id: String, name: String, items: [Item], depth: Int) -> Folder {
        var videos: [Item] = []
        var children: [String: [Item]] = [:]
        for item in items {
            let comps = item.folderComponents
            if comps.count > depth {
                children[comps[depth], default: []].append(item)
            } else {
                videos.append(item)
            }
        }
        // Folders first, then videos, each alphabetical (LibraryRow rules).
        let folders = children.keys
            .sorted { $0.localizedStandardCompare($1) == .orderedAscending }
            .map { build(id: id + "/" + $0, name: $0, items: children[$0]!, depth: depth + 1) }
        videos.sort { $0.name.localizedStandardCompare($1.name) == .orderedAscending }
        return Folder(id: id, name: name, folders: folders, videos: videos, count: items.count)
    }
}

import Foundation
import Observation

enum ConnectionState: Equatable {
    case connecting
    case connected(name: String, version: String)
    case offline(String)
}

/// App-wide state: which server, whether it answers, the library and resume positions.
@MainActor
@Observable
final class AppModel {
    private(set) var client: RiffleClient?
    private(set) var connection: ConnectionState = .connecting
    private(set) var library: LibraryResponse?
    private(set) var roots: [Folder] = []
    private(set) var progress: [String: WatchProgress] = [:] { didSet { recount() } }
    /// Watch-state counts per folder id ("Movies", "Movies/Pixar"), every level included.
    private(set) var folderCounts: [String: WatchCounts] = [:]
    private(set) var lastError: String?

    private var itemsByID: [String: Item] = [:]
    private var libraryETag: String?
    private var progressETag: String?

    private static let addressKey = "server.address"
    private static let tokenAccount = "server.token"

    init() {
        if let saved = UserDefaults.standard.string(forKey: Self.addressKey),
           let url = URL(string: saved) {
            client = RiffleClient(baseURL: url, token: Keychain.read(Self.tokenAccount) ?? "")
            if let cached = Self.loadCachedLibrary() { apply(cached) }
        }
    }

    var isConfigured: Bool { client != nil }
    var serverAddress: String? { client?.baseURL.absoluteString }

    func item(_ id: String) -> Item? { itemsByID[id] }

    func state(_ id: String) -> WatchState { progress[id]?.state ?? .unwatched }

    func counts(_ folder: Folder) -> WatchCounts { folderCounts[folder.id] ?? WatchCounts() }

    /// Started videos, most recently watched first.
    var continueWatching: [Item] {
        progress
            .filter { $0.value.isInProgress && itemsByID[$0.key] != nil }
            .sorted { ($0.value.updatedAt ?? .distantPast) > ($1.value.updatedAt ?? .distantPast) }
            .compactMap { itemsByID[$0.key] }
            .prefix(12)
            .map { $0 }
    }

    // MARK: Connecting

    /// Checks the address and token against the server, then saves them.
    func connect(address: String, token: String) async throws {
        guard let url = RiffleClient.normalize(address) else { throw RiffleError.invalidAddress }
        let candidate = RiffleClient(baseURL: url, token: token.trimmingCharacters(in: .whitespacesAndNewlines))
        let health = try await candidate.health()
        // /api/health is open; the library proves the token works.
        guard case .fresh(let lib, let etag) = try await candidate.library(etag: nil) else {
            throw RiffleError.badResponse
        }
        UserDefaults.standard.set(url.absoluteString, forKey: Self.addressKey)
        Keychain.write(Self.tokenAccount, candidate.token)
        client = candidate
        libraryETag = etag
        progressETag = nil
        apply(lib)
        Self.saveCachedLibrary(lib)
        connection = .connected(name: Self.displayName(health), version: health.version)
        await refreshProgress()
    }

    func disconnect() {
        UserDefaults.standard.removeObject(forKey: Self.addressKey)
        Keychain.write(Self.tokenAccount, nil)
        Self.saveCachedLibrary(nil)
        client = nil
        library = nil
        roots = []
        itemsByID = [:]
        progress = [:]
        libraryETag = nil
        progressETag = nil
        connection = .connecting
    }

    // MARK: Refreshing

    /// Re-reads health, library and progress. Cheap when nothing changed (ETags).
    func refresh() async {
        guard let client else { return }
        if case .offline = connection { connection = .connecting }
        do {
            let health = try await client.health()
            connection = .connected(name: Self.displayName(health), version: health.version)
            if case .fresh(let lib, let etag) = try await client.library(etag: libraryETag) {
                libraryETag = etag
                apply(lib)
                Self.saveCachedLibrary(lib)
            }
            await refreshProgress()
            lastError = nil
        } catch is CancellationError {
        } catch {
            connection = .offline(error.localizedDescription)
            lastError = error.localizedDescription
        }
    }

    func refreshProgress() async {
        guard let client else { return }
        if case .fresh(let p, let etag) = try? await client.progress(etag: progressETag) {
            progress = p
            progressETag = etag
        }
    }

    func rescan() async throws {
        try await client?.rescan()
    }

    /// Records a position locally right away and sends it to the server, which
    /// answers with the entry and its watch state.
    func saveProgress(_ id: String, position: Double, duration: Double) async {
        var local = progress[id] ?? WatchProgress(position: position, duration: duration)
        local.position = position
        local.duration = duration
        local.updatedAt = .now
        progress[id] = local
        progressETag = nil
        if let entry = try? await client?.putProgress(id, position: position, duration: duration) {
            progress[id] = entry
        }
    }

    /// Mark as watched or unwatched. Updates at once; puts it back if the server refuses.
    func setWatched(_ id: String, _ watched: Bool) async {
        let before = progress[id]
        progress[id] = watched
            ? WatchProgress(position: 0, duration: before?.duration ?? item(id)?.duration ?? 0,
                            updatedAt: .now, state: .watched)
            : nil
        progressETag = nil
        do {
            progress[id] = try await client?.setWatched(id, watched)
        } catch {
            progress[id] = before
            lastError = error.localizedDescription
        }
    }

    // MARK: Private

    private func apply(_ lib: LibraryResponse) {
        library = lib
        itemsByID = Dictionary(lib.items.map { ($0.id, $0) }, uniquingKeysWith: { a, _ in a })
        roots = Folder.roots(from: lib)
        recount()
    }

    /// Counts per folder are the client's job (server/README.md, Watch state): each
    /// item adds its state to its root and every folder above it.
    private func recount() {
        var counts: [String: WatchCounts] = [:]
        for item in library?.items ?? [] {
            let state = progress[item.id]?.state ?? .unwatched
            var id = item.root
            counts[id, default: WatchCounts()].add(state)
            for component in item.folderComponents {
                id += "/" + component
                counts[id, default: WatchCounts()].add(state)
            }
        }
        folderCounts = counts
    }

    private static func displayName(_ h: HealthInfo) -> String {
        h.name == "riffle-server" || h.name.isEmpty ? "Riffle Server" : h.name
    }

    // The last library is kept on disk so the app opens instantly, and offline.
    private static var cacheURL: URL {
        URL.cachesDirectory.appending(path: "library.json")
    }

    private static func loadCachedLibrary() -> LibraryResponse? {
        guard let data = try? Data(contentsOf: cacheURL) else { return nil }
        return try? RiffleClient.decoder.decode(LibraryResponse.self, from: data)
    }

    private static func saveCachedLibrary(_ lib: LibraryResponse?) {
        guard let lib else {
            try? FileManager.default.removeItem(at: cacheURL)
            return
        }
        if let data = try? RiffleClient.encoder.encode(lib) {
            try? data.write(to: cacheURL, options: .atomic)
        }
    }
}

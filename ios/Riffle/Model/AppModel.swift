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
    private(set) var progress: [String: Progress] = [:]
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

    /// Started videos, most recently watched first.
    var continueWatching: [Item] {
        progress
            .filter { $0.value.isResumable }
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

    /// Records a position locally right away and sends it to the server.
    func saveProgress(_ id: String, position: Double, duration: Double) async {
        progress[id] = Progress(position: position, duration: duration, updatedAt: .now)
        progressETag = nil
        try? await client?.putProgress(id, position: position, duration: duration)
    }

    // MARK: Private

    private func apply(_ lib: LibraryResponse) {
        library = lib
        itemsByID = Dictionary(lib.items.map { ($0.id, $0) }, uniquingKeysWith: { a, _ in a })
        roots = Folder.roots(from: lib)
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

import UIKit

/// Library stills from `/api/items/{id}/poster.jpg`, kept in memory and on disk.
actor PosterCache {
    static let shared = PosterCache()

    private let memory = NSCache<NSString, UIImage>()
    private let dir: URL = {
        let d = URL.cachesDirectory.appending(path: "posters", directoryHint: .isDirectory)
        try? FileManager.default.createDirectory(at: d, withIntermediateDirectories: true)
        return d
    }()

    init() { memory.countLimit = 300 }

    func cached(_ id: String) -> UIImage? {
        if let img = memory.object(forKey: id as NSString) { return img }
        guard let data = try? Data(contentsOf: dir.appending(path: "\(id).jpg")),
              let img = UIImage(data: data) else { return nil }
        memory.setObject(img, forKey: id as NSString)
        return img
    }

    /// Fetches the poster. `.pending` while the server is still making trickplay
    /// (asking for the poster doesn't move the item up the queue).
    func load(_ id: String, client: RiffleClient) async throws -> Pending<UIImage> {
        if let img = cached(id) { return .ready(img) }
        switch try await client.poster(id) {
        case .ready(let data):
            guard let raw = UIImage(data: data) else { return .unavailable }
            let img = await raw.byPreparingForDisplay() ?? raw
            memory.setObject(img, forKey: id as NSString)
            try? data.write(to: dir.appending(path: "\(id).jpg"), options: .atomic)
            return .ready(img)
        case .pending(let retry): return .pending(retryAfter: retry)
        case .unavailable: return .unavailable
        }
    }
}

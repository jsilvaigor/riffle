import Foundation

// Wire types for the Riffle Server API (`server/README.md`). Go encodes empty slices
// as `null`, so every array is optional on the wire and exposed non-optional here.

struct HealthInfo: Decodable, Sendable {
    var name: String
    var version: String
    var commit: String?
    var authRequired: Bool
}

struct LibraryResponse: Codable, Sendable {
    var roots: [String]
    var items: [Item]

    private enum CodingKeys: String, CodingKey { case roots, items }

    init(roots: [String], items: [Item]) {
        self.roots = roots
        self.items = items
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        roots = try c.decodeIfPresent([String].self, forKey: .roots) ?? []
        items = try c.decodeIfPresent([Item].self, forKey: .items) ?? []
    }
}

enum TrickplayState: String, Codable, Sendable {
    case none, pending, ready, failed

    init(from decoder: Decoder) throws {
        let raw = try decoder.singleValueContainer().decode(String.self)
        self = TrickplayState(rawValue: raw) ?? .none
    }
}

struct Item: Codable, Identifiable, Hashable, Sendable {
    var id: String
    var root: String
    /// Slash-separated, relative to the root, file name included.
    var path: String
    var name: String
    var size: Int64
    var modTime: Date?
    var probe: Probe?
    var probeError: String?
    var trickplay: TrickplayState
    var trickplayError: String?

    /// Folder components between the root and the file.
    var folderComponents: [String] {
        Array(path.split(separator: "/").dropLast().map(String.init))
    }

    /// The file name without its extension, for cards.
    var displayTitle: String {
        (name as NSString).deletingPathExtension
    }

    var duration: Double? { probe?.duration }
}

struct Probe: Codable, Hashable, Sendable {
    var duration: Double
    var container: String?
    var bitrate: Int64?
    var video: VideoStream?
    var audio: [AudioStream]
    var subtitles: [SubStream]

    private enum CodingKeys: String, CodingKey { case duration, container, bitrate, video, audio, subtitles }

    init(duration: Double, container: String? = nil, bitrate: Int64? = nil, video: VideoStream? = nil,
         audio: [AudioStream] = [], subtitles: [SubStream] = []) {
        self.duration = duration
        self.container = container
        self.bitrate = bitrate
        self.video = video
        self.audio = audio
        self.subtitles = subtitles
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        duration = try c.decodeIfPresent(Double.self, forKey: .duration) ?? 0
        container = try c.decodeIfPresent(String.self, forKey: .container)
        bitrate = try c.decodeIfPresent(Int64.self, forKey: .bitrate)
        video = try c.decodeIfPresent(VideoStream.self, forKey: .video)
        audio = try c.decodeIfPresent([AudioStream].self, forKey: .audio) ?? []
        subtitles = try c.decodeIfPresent([SubStream].self, forKey: .subtitles) ?? []
    }
}

struct VideoStream: Codable, Hashable, Sendable {
    var index: Int
    var codec: String
    var profile: String?
    var width: Int
    var height: Int
    var frameRate: Double?
    var pixFmt: String?
    /// Degrees the stored frame turns clockwise to display.
    var rotation: Int?
    /// Size after rotation and non-square pixels; 0 or absent from older servers.
    var displayWidth: Int?
    var displayHeight: Int?

    /// Display width ÷ height, falling back to the stored size with rotation applied.
    var displayAspect: Double? {
        if let w = displayWidth, let h = displayHeight, w > 0, h > 0 {
            return Double(w) / Double(h)
        }
        guard width > 0, height > 0 else { return nil }
        let quarterTurn = (rotation ?? 0) % 180 != 0
        return quarterTurn ? Double(height) / Double(width) : Double(width) / Double(height)
    }
}

struct AudioStream: Codable, Hashable, Sendable {
    var index: Int
    var codec: String
    var channels: Int
    var language: String?
    var title: String?
    var `default`: Bool?
}

struct SubStream: Codable, Hashable, Sendable {
    var index: Int
    var codec: String
    var language: String?
    var title: String?
    var `default`: Bool?
    var forced: Bool?
}

struct WatchProgress: Codable, Hashable, Sendable {
    var position: Double
    var duration: Double
    var updatedAt: Date?

    /// 0–1 for progress bars.
    var fraction: Double {
        duration > 0 ? min(max(position / duration, 0), 1) : 0
    }

    /// Worth offering "Resume": started, and not within the last 5% (or 30 s).
    var isResumable: Bool {
        position >= 10 && position < duration - max(30, duration * 0.05)
    }
}

/// `GET /api/items/{id}/trickplay/manifest.json`. A contract with the server:
/// 10x10 sheets, thumbnail `k` at time `k * interval`.
struct TrickplayManifest: Codable, Hashable, Sendable {
    var interval: Double
    var tileWidth: Int
    var tileHeight: Int
    var columns: Int
    var rows: Int
    var count: Int
    var sheets: Int
    var format: String
}

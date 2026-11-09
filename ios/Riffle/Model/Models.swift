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

/// The server decides each item's watch state (server/README.md, Watch state), so the
/// app and the dashboard agree. No progress entry means unwatched.
enum WatchState: String, Codable, Sendable {
    case unwatched, partial, watched

    init(from decoder: Decoder) throws {
        let raw = try decoder.singleValueContainer().decode(String.self)
        self = WatchState(rawValue: raw) ?? .unwatched
    }
}

struct WatchProgress: Codable, Hashable, Sendable {
    var position: Double
    var duration: Double
    var updatedAt: Date?
    var state: WatchState
    /// Last time it became watched, and how many times that happened.
    var finishedAt: Date?
    var timesFinished: Int?

    init(position: Double, duration: Double, updatedAt: Date? = nil, state: WatchState = .partial,
         finishedAt: Date? = nil, timesFinished: Int? = nil) {
        self.position = position
        self.duration = duration
        self.updatedAt = updatedAt
        self.state = state
        self.finishedAt = finishedAt
        self.timesFinished = timesFinished
    }

    private enum CodingKeys: String, CodingKey {
        case position, duration, updatedAt, state, finishedAt, timesFinished
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        position = try c.decodeIfPresent(Double.self, forKey: .position) ?? 0
        duration = try c.decodeIfPresent(Double.self, forKey: .duration) ?? 0
        updatedAt = try c.decodeIfPresent(Date.self, forKey: .updatedAt)
        finishedAt = try c.decodeIfPresent(Date.self, forKey: .finishedAt)
        timesFinished = try c.decodeIfPresent(Int.self, forKey: .timesFinished)
        // Servers before 0.6.0 send no state: an entry means it was started.
        state = try c.decodeIfPresent(WatchState.self, forKey: .state) ?? .partial
    }

    /// 0–1 for progress bars.
    var fraction: Double {
        duration > 0 ? min(max(position / duration, 0), 1) : 0
    }

    /// Worth offering "Resume": started, and not within the last 5% (or 30 s).
    var isResumable: Bool {
        position >= 10 && position < duration - max(30, duration * 0.05)
    }

    /// Shows a progress bar and belongs in Continue watching: in progress, or a
    /// watched video being watched again (watched is sticky on the server).
    var isInProgress: Bool {
        state == .partial || (state == .watched && isResumable)
    }
}

/// How many videos in a folder (and the folders below it) are in each state.
struct WatchCounts: Hashable, Sendable {
    var watched = 0
    var partial = 0
    var unwatched = 0

    var total: Int { watched + partial + unwatched }

    mutating func add(_ state: WatchState) {
        switch state {
        case .watched: watched += 1
        case .partial: partial += 1
        case .unwatched: unwatched += 1
        }
    }

    /// "14 watched · 2 in progress · 8 not watched", leaving out empty parts.
    var summary: String {
        guard total > 0 else { return "No videos" }
        if watched == total { return "All watched" }
        if unwatched == total { return "Not watched" }
        return [
            watched > 0 ? "\(watched) watched" : nil,
            partial > 0 ? "\(partial) in progress" : nil,
            unwatched > 0 ? "\(unwatched) not watched" : nil,
        ].compactMap { $0 }.joined(separator: " · ")
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
    /// The picture area when the file has black bands baked in; nil when it has none.
    var crop: TrickplayCrop?
}

/// A rectangle in the decoded frame's stored pixels (`frameWidth` × `frameHeight`),
/// the space mpv's `video-crop` uses. The tiles show the whole frame.
struct TrickplayCrop: Codable, Hashable, Sendable {
    var x: Int
    var y: Int
    var w: Int
    var h: Int
    var frameWidth: Int
    var frameHeight: Int

    /// mpv's `video-crop` syntax: `WxH+X+Y`.
    var mpvValue: String { "\(w)x\(h)+\(x)+\(y)" }
}

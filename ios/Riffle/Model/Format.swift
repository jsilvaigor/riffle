import Foundation

/// Text formatting rules from `design/README.md` (Content fundamentals).
enum Format {
    /// `m:ss` under an hour, `h:mm:ss` over it.
    static func time(_ seconds: Double) -> String {
        let total = max(0, Int(seconds.isFinite ? seconds.rounded(.down) : 0))
        let h = total / 3600, m = (total % 3600) / 60, s = total % 60
        return h > 0
            ? String(format: "%d:%02d:%02d", h, m, s)
            : String(format: "%d:%02d", m, s)
    }

    /// Remaining time with a true minus sign: "−12:40".
    static func remaining(_ seconds: Double) -> String {
        "\u{2212}" + time(seconds)
    }

    static func size(_ bytes: Int64) -> String {
        ByteCountFormatter.string(fromByteCount: bytes, countStyle: .file)
    }

    /// "1:52:10 · 4.2 GB"
    static func meta(_ item: Item) -> String {
        var parts: [String] = []
        if let d = item.duration, d > 0 { parts.append(time(d)) }
        parts.append(size(item.size))
        return parts.joined(separator: " · ")
    }

    /// Resolution from the display size's short side.
    static func resolution(_ v: VideoStream) -> String? {
        let w = v.displayWidth.flatMap { $0 > 0 ? $0 : nil } ?? v.width
        let h = v.displayHeight.flatMap { $0 > 0 ? $0 : nil } ?? v.height
        let short = min(w, h), long = max(w, h)
        switch (short, long) {
        case _ where long >= 3200 || short >= 2000: return "4K"
        case _ where short >= 1400: return "1440p"
        case _ where short >= 1000 || long >= 1900: return "1080p"
        case _ where short >= 700 || long >= 1260: return "720p"
        case _ where short > 0: return "SD"
        default: return nil
        }
    }

    static func videoCodec(_ codec: String) -> String {
        switch codec.lowercased() {
        case "h264": "H.264"
        case "hevc", "h265": "HEVC"
        case "av1": "AV1"
        case "vp9": "VP9"
        case "vp8": "VP8"
        case "mpeg4": "MPEG-4"
        case "mpeg2video": "MPEG-2"
        case "mpeg1video": "MPEG-1"
        case let c where c.hasPrefix("wmv") || c.hasPrefix("vc1"): "WMV"
        default: codec.uppercased()
        }
    }

    static func audioCodec(_ codec: String) -> String {
        switch codec.lowercased() {
        case "aac": "AAC"
        case "ac3": "AC3"
        case "eac3": "E-AC3"
        case "dts": "DTS"
        case "truehd": "TrueHD"
        case "flac": "FLAC"
        case "opus": "Opus"
        case "mp3": "MP3"
        case let c where c.hasPrefix("pcm"): "PCM"
        case let c where c.hasPrefix("wma"): "WMA"
        default: codec.uppercased()
        }
    }

    static func channels(_ n: Int) -> String {
        switch n {
        case 1: "Mono"
        case 2: "Stereo"
        case 6: "5.1"
        case 8: "7.1"
        default: "\(n) ch"
        }
    }

    /// Badges for a video, most important first: resolution, then codec when it isn't
    /// the everyday H.264. At most `limit` (two on thumbnails, three in rows).
    static func badges(_ item: Item, limit: Int) -> [String] {
        guard let probe = item.probe else { return [] }
        var out: [String] = []
        if let v = probe.video {
            if let r = resolution(v) { out.append(r) }
            if v.codec.lowercased() != "h264" { out.append(videoCodec(v.codec)) }
        }
        if let a = probe.audio.first, a.channels >= 6 { out.append(channels(a.channels)) }
        if !probe.subtitles.isEmpty { out.append("CC") }
        return Array(out.prefix(limit))
    }

    /// A language tag in its own name: "pt-BR" → "Português (Brasil)".
    static func language(_ code: String?) -> String? {
        guard let code, !code.isEmpty, code != "und" else { return nil }
        let locale = Locale(identifier: code)
        let name = locale.localizedString(forIdentifier: code)
        return name.map { $0.prefix(1).uppercased() + $0.dropFirst() } ?? code
    }
}

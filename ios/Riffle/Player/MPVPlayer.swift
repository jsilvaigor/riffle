import Foundation
import Libmpv
import QuartzCore
import UIKit

/// What mpv tells the app. Delivered on the main actor.
enum MPVEvent: Sendable {
    case timePos(Double)
    case duration(Double)
    case paused(Bool)
    case pausedForCache(Bool)
    case seeking(Bool)
    /// The furthest point the demuxer has buffered, in seconds.
    case cacheEnd(Double)
    case eofReached(Bool)
    /// Display size after rotation and pixel aspect.
    case videoSize(width: Int, height: Int)
    case audioTrack(String)
    case subtitleTrack(String)
    case fileLoaded
    /// A seek (or the start) finished and frames are flowing again.
    case playbackRestart
    case endFile(error: String?)
}

/// A thin, thread-safe wrapper around one libmpv instance rendering into a
/// `CAMetalLayer` (gpu-next over MoltenVK, VideoToolbox decoding).
///
/// Commands may be sent from any thread; they're asynchronous so the main thread
/// never waits on mpv's core lock, which matters while scrubbing.
final class MPVPlayer: @unchecked Sendable {
    private var mpv: OpaquePointer?
    private let queue = DispatchQueue(label: "riffle.mpv.events", qos: .userInitiated)
    private let layer: CAMetalLayer
    private let onEvent: @MainActor (MPVEvent) -> Void

    private static let observed: [(name: String, format: mpv_format)] = [
        ("time-pos", MPV_FORMAT_DOUBLE),
        ("duration", MPV_FORMAT_DOUBLE),
        ("pause", MPV_FORMAT_FLAG),
        ("paused-for-cache", MPV_FORMAT_FLAG),
        ("seeking", MPV_FORMAT_FLAG),
        ("demuxer-cache-time", MPV_FORMAT_DOUBLE),
        ("eof-reached", MPV_FORMAT_FLAG),
        ("dwidth", MPV_FORMAT_INT64),
        ("dheight", MPV_FORMAT_INT64),
        ("aid", MPV_FORMAT_STRING),
        ("sid", MPV_FORMAT_STRING),
    ]

    private var lastWidth = 0, lastHeight = 0

    init(layer: CAMetalLayer, headers: [String: String], onEvent: @escaping @MainActor (MPVEvent) -> Void) {
        self.layer = layer
        self.onEvent = onEvent
        guard let mpv = mpv_create() else { return }
        self.mpv = mpv

        var wid = Int64(Int(bitPattern: Unmanaged.passUnretained(layer).toOpaque()))
        mpv_set_option(mpv, "wid", MPV_FORMAT_INT64, &wid)

        let options: [(String, String)] = [
            ("vo", "gpu-next"),
            ("gpu-api", "vulkan"),
            ("gpu-context", "moltenvk"),
            ("hwdec", "videotoolbox"),
            // The last frame stays up at the end (player.md, Ended).
            ("keep-open", "yes"),
            ("idle", "yes"),
            // A big back buffer makes scrubbing backwards instant; readahead hides
            // HDD hiccups. Seeks inside the cache never touch the network.
            ("cache", "yes"),
            ("demuxer-max-bytes", "256MiB"),
            ("demuxer-max-back-bytes", "128MiB"),
            ("demuxer-readahead-secs", "30"),
            // Spun-down array disks take 5-10 s; give them room before failing.
            ("network-timeout", "30"),
            ("user-agent", "Riffle"),
            ("http-header-fields", headers.map { "\($0.key): \($0.value)" }.joined(separator: ",")),
            ("subs-match-os-language", "yes"),
            ("subs-fallback", "default"),
            ("input-default-bindings", "no"),
            ("terminal", "no"),
            ("ytdl", "no"),
        ]
        for (k, v) in options { mpv_set_option_string(mpv, k, v) }
        #if DEBUG
        mpv_request_log_messages(mpv, "warn")
        #endif

        guard mpv_initialize(mpv) >= 0 else {
            mpv_terminate_destroy(mpv)
            self.mpv = nil
            return
        }
        for (i, p) in Self.observed.enumerated() {
            mpv_observe_property(mpv, UInt64(i), p.name, p.format)
        }
        mpv_set_wakeup_callback(mpv, { ctx in
            guard let ctx else { return }
            Unmanaged<MPVPlayer>.fromOpaque(ctx).takeUnretainedValue().drain()
        }, Unmanaged.passUnretained(self).toOpaque())
    }

    var isAvailable: Bool { mpv != nil }

    // MARK: Commands

    func load(_ url: URL, start: Double) {
        var opts: [String] = []
        if start > 0 { opts.append("start=\(String(format: "%.3f", start))") }
        command(["loadfile", url.absoluteString, "replace", "-1", opts.joined(separator: ",")])
    }

    enum SeekMode: String { case keyframe = "absolute+keyframes", exact = "absolute+exact" }

    func seek(to seconds: Double, mode: SeekMode) {
        command(["seek", String(format: "%.3f", max(0, seconds)), mode.rawValue])
    }

    func setPaused(_ paused: Bool) { set("pause", paused ? "yes" : "no") }
    func setSpeed(_ speed: Double) { set("speed", String(format: "%.2f", speed)) }
    /// 0 fits the whole frame, 1 fills the screen (cropping).
    func setFill(_ fill: Bool) { set("panscan", fill ? "1.0" : "0.0") }
    func setAudioTrack(_ id: String) { set("aid", id) }
    func setSubtitleTrack(_ id: String) { set("sid", id) }

    /// Tells mpv the layer changed size (rotation). MPVKit's MoltenVK context only
    /// reads the drawable size when the video output reconfigures, which happens
    /// only when the video parameters change. So set the aspect override to the
    /// video's own aspect, nudged by 0.001% (alternating up and down so it never
    /// drifts): a reconfigure with no visible change.
    func layerDidResize() {
        guard let mpv else { return }
        var aspect = 0.0
        guard mpv_get_property(mpv, "video-params/aspect", MPV_FORMAT_DOUBLE, &aspect) >= 0, aspect > 0 else { return }
        aspectNudgeUp.toggle()
        set("video-aspect-override", String(format: "%.8f", aspect * (aspectNudgeUp ? 1.00001 : 0.99999)))
    }

    private var aspectNudgeUp = false

    /// Stops decoding while the app is in the background (and back), so the Metal
    /// layer isn't drawn to off screen.
    func setVideoEnabled(_ on: Bool) { set("vid", on ? "auto" : "no") }

    /// Tears mpv down. The instance is unusable afterwards.
    func destroy() {
        guard let mpv else { return }
        self.mpv = nil
        mpv_set_wakeup_callback(mpv, nil, nil)
        let layer = self.layer  // keep the layer alive until mpv lets go of it
        queue.async {
            mpv_terminate_destroy(mpv)
            _ = layer
        }
    }

    deinit { destroy() }

    private func set(_ name: String, _ value: String) {
        guard let mpv else { return }
        // mpv copies the value before returning.
        value.withCString { cstr in
            var p: UnsafePointer<CChar>? = cstr
            _ = mpv_set_property_async(mpv, 0, name, MPV_FORMAT_STRING, &p)
        }
    }

    private func command(_ args: [String]) {
        guard let mpv else { return }
        var cargs: [UnsafePointer<CChar>?] = args.map { UnsafePointer(strdup($0)) }
        cargs.append(nil)
        defer { for p in cargs where p != nil { free(UnsafeMutablePointer(mutating: p)) } }
        mpv_command_async(mpv, 0, &cargs)
    }

    // MARK: Events

    private func drain() {
        queue.async { [weak self] in
            guard let self else { return }
            while let mpv = self.mpv, let event = mpv_wait_event(mpv, 0) {
                let id = event.pointee.event_id
                if id == MPV_EVENT_NONE { break }
                if let out = self.translate(event.pointee) {
                    let onEvent = self.onEvent
                    DispatchQueue.main.async { MainActor.assumeIsolated { onEvent(out) } }
                }
                if id == MPV_EVENT_SHUTDOWN { break }
            }
        }
    }

    private func translate(_ e: mpv_event) -> MPVEvent? {
        switch e.event_id {
        case MPV_EVENT_PROPERTY_CHANGE:
            guard let prop = e.data?.assumingMemoryBound(to: mpv_event_property.self).pointee,
                  Int(e.reply_userdata) < Self.observed.count, let data = prop.data else { return nil }
            let name = Self.observed[Int(e.reply_userdata)].name
            switch prop.format {
            case MPV_FORMAT_DOUBLE:
                let v = data.assumingMemoryBound(to: Double.self).pointee
                switch name {
                case "time-pos": return .timePos(v)
                case "duration": return .duration(v)
                case "demuxer-cache-time": return .cacheEnd(v)
                default: return nil
                }
            case MPV_FORMAT_FLAG:
                let v = data.assumingMemoryBound(to: Int32.self).pointee != 0
                switch name {
                case "pause": return .paused(v)
                case "paused-for-cache": return .pausedForCache(v)
                case "seeking": return .seeking(v)
                case "eof-reached": return .eofReached(v)
                default: return nil
                }
            case MPV_FORMAT_INT64:
                let v = Int(data.assumingMemoryBound(to: Int64.self).pointee)
                if name == "dwidth" { lastWidth = v } else { lastHeight = v }
                guard lastWidth > 0, lastHeight > 0 else { return nil }
                return .videoSize(width: lastWidth, height: lastHeight)
            case MPV_FORMAT_STRING:
                guard let cstr = data.assumingMemoryBound(to: UnsafePointer<CChar>?.self).pointee else { return nil }
                let v = String(cString: cstr)
                return name == "aid" ? .audioTrack(v) : .subtitleTrack(v)
            default:
                return nil
            }
        case MPV_EVENT_FILE_LOADED:
            return .fileLoaded
        case MPV_EVENT_PLAYBACK_RESTART:
            return .playbackRestart
        case MPV_EVENT_END_FILE:
            guard let end = e.data?.assumingMemoryBound(to: mpv_event_end_file.self).pointee else { return nil }
            if end.reason == MPV_END_FILE_REASON_ERROR {
                return .endFile(error: String(cString: mpv_error_string(end.error)))
            }
            return end.reason == MPV_END_FILE_REASON_EOF ? .endFile(error: nil) : nil
        case MPV_EVENT_LOG_MESSAGE:
            #if DEBUG
            if let msg = e.data?.assumingMemoryBound(to: mpv_event_log_message.self).pointee {
                print("[mpv] \(String(cString: msg.prefix)): \(String(cString: msg.text))", terminator: "")
            }
            #endif
            return nil
        default:
            return nil
        }
    }
}

/// MoltenVK briefly sets a 1x1 drawable to force a present, which flickers and can
/// stick; ignore those sizes (mpv-player/mpv#13651, as in MPVKit's demo).
final class MPVMetalLayer: CAMetalLayer {
    override var drawableSize: CGSize {
        get { super.drawableSize }
        set {
            if Int(newValue.width) > 1 && Int(newValue.height) > 1 { super.drawableSize = newValue }
        }
    }
}

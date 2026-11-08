import AVFoundation
import Observation
import SwiftUI
import UIKit

/// Everything the player screen shows and does, on top of one `MPVPlayer`.
/// Behaviour follows `design/player.md`.
@MainActor
@Observable
final class PlayerModel {
    enum Phase: Equatable {
        /// Waiting for the first frame.
        case opening
        case playing
        case ended
        case failed(title: String, message: String)
    }

    enum ScrubSpeed: Equatable {
        case full, half, quarter, fine

        var factor: Double {
            switch self { case .full: 1; case .half: 0.5; case .quarter: 0.25; case .fine: 0.125 }
        }

        /// Up to 56 pt above the track is full speed, then half, quarter and fine.
        init(heightAboveTrack h: CGFloat) {
            switch h {
            case ..<56: self = .full
            case ..<112: self = .half
            case ..<168: self = .quarter
            default: self = .fine
            }
        }

        var label: String? {
            switch self {
            case .full: nil
            case .half: "Half-speed scrubbing"
            case .quarter: "Quarter-speed scrubbing"
            case .fine: "Fine scrubbing"
            }
        }
    }

    let request: PlaybackRequest
    let trickplay = TrickplayStore()
    let layer = MPVMetalLayer()

    // Playback state from mpv.
    private(set) var phase: Phase = .opening
    private(set) var position: Double
    private(set) var duration: Double
    private(set) var buffered: Double = 0
    private(set) var isPaused = false
    private(set) var isBuffering = true
    private(set) var audioTrack = "auto"
    private(set) var subtitleTrack = "no"
    private(set) var displayAspect: Double?

    // Interface state.
    private(set) var chromeVisible = true
    private(set) var scrubTime: Double?
    private(set) var scrubSpeed: ScrubSpeed = .full
    private(set) var skipLabel: String?
    private(set) var isFastForwarding = false
    private(set) var fill = false
    private(set) var rotationLocked = false
    /// "Waking up the disk…" or nil; shown under the spinner.
    private(set) var statusText: String?
    /// Buffering for 15 s: show "Still waiting for Riffle Server".
    private(set) var stalled = false
    /// The opening spinner appears only after 400 ms so fast opens don't flash.
    private(set) var showSpinner = false
    var sheetOpen = false { didSet { pokeChrome() } }

    @ObservationIgnored private var player: MPVPlayer?
    @ObservationIgnored private let model: AppModel
    @ObservationIgnored private var hasStarted = false
    @ObservationIgnored private var layerReady = false
    @ObservationIgnored private var orientationDecided = false
    @ObservationIgnored private var tasks: [Task<Void, Never>] = []
    @ObservationIgnored private var hideTask: Task<Void, Never>?
    @ObservationIgnored private var bufferTask: Task<Void, Never>?
    @ObservationIgnored private var skipTask: Task<Void, Never>?
    @ObservationIgnored private var skipTotal = 0.0

    // Scrub seeking: at most one keyframe seek in flight, no more often than 120 ms.
    @ObservationIgnored private var seekInFlight = false
    @ObservationIgnored private var lastSeekAt = Date.distantPast
    @ObservationIgnored private var pendingScrubSeek: Double?
    @ObservationIgnored private var exactSeekAfterRestart: Double?
    @ObservationIgnored private var lastReported: Double = -1

    init(request: PlaybackRequest, model: AppModel) {
        self.request = request
        self.model = model
        self.position = request.start
        self.duration = request.item.duration ?? 0
        self.displayAspect = request.item.probe?.video?.displayAspect
    }

    var title: String { request.item.displayTitle }

    /// "Movies · HEVC 1080p · AC3 5.1"
    var subtitle: String {
        var parts = [request.item.root]
        if let v = request.item.probe?.video {
            parts.append([Format.videoCodec(v.codec), Format.resolution(v)].compactMap { $0 }.joined(separator: " "))
        }
        if let a = request.item.probe?.audio.first {
            parts.append("\(Format.audioCodec(a.codec)) \(Format.channels(a.channels))")
        }
        return parts.joined(separator: " · ")
    }

    /// What the scrubber and readouts show: the finger while scrubbing, else playback.
    var displayedTime: Double { scrubTime ?? position }

    // MARK: Lifecycle

    func start() {
        guard player == nil, let client = model.client else { return }
        try? AVAudioSession.sharedInstance().setCategory(.playback, mode: .moviePlayback)
        try? AVAudioSession.sharedInstance().setActive(true)
        decideOrientation()

        let player = MPVPlayer(layer: layer, headers: client.streamHeaders) { [weak self] event in
            self?.handle(event)
        }
        self.player = player
        guard player.isAvailable else {
            phase = .failed(title: "Couldn't start the player",
                            message: "Riffle couldn't set up video playback on this device.")
            return
        }
        beginBufferWatch(firstOpen: true)

        tasks.append(Task { [weak self] in
            try? await Task.sleep(for: .milliseconds(400))
            self?.showSpinner = true
        })
        tasks.append(Task { [trickplay, request] in
            await trickplay.load(itemID: request.item.id, client: client)
        })
        // Resume position every 10 seconds while playing (player.md).
        tasks.append(Task { [weak self] in
            while !Task.isCancelled {
                try? await Task.sleep(for: .seconds(10))
                await self?.reportProgress()
            }
        })
        observeAppState()
    }

    /// The video layer is on screen with its final size: start loading.
    func videoLayerReady() {
        guard !layerReady, let client = model.client else { return }
        layerReady = true
        player?.load(client.streamURL(request.item.id), start: request.start)
    }

    func videoLayerResized() {
        player?.layerDidResize()
    }

    /// Called as the player closes: saves the position and releases mpv.
    func stop() async {
        tasks.forEach { $0.cancel() }
        hideTask?.cancel()
        bufferTask?.cancel()
        await reportProgress(force: true)
        player?.destroy()
        player = nil
        Orientation.exitPlayer()
        try? AVAudioSession.sharedInstance().setActive(false, options: .notifyOthersOnDeactivation)
    }

    func retry() {
        guard let client = model.client else { return }
        phase = .opening
        stalled = false
        player?.load(client.streamURL(request.item.id), start: position)
        beginBufferWatch(firstOpen: false)
    }

    // MARK: Transport

    func togglePlay() {
        if phase == .ended { replay(); return }
        player?.setPaused(!isPaused)
        pokeChrome()
    }

    func replay() {
        phase = .playing
        player?.seek(to: 0, mode: .keyframe)
        player?.setPaused(false)
        pokeChrome()
    }

    func skip(_ seconds: Double) {
        let target = min(max(position + seconds, 0), max(duration, 0))
        position = target
        player?.seek(to: target, mode: .exact)
        if phase == .ended && seconds < 0 { phase = .playing }
        pokeChrome()
    }

    /// Double-tap skipping: each extra tap within 0.6 s adds 10 s; a label shows the total.
    func tapSkip(forward: Bool, continuing: Bool) {
        let step = forward ? 10.0 : -10.0
        skipTotal = continuing && (skipTotal > 0) == forward ? skipTotal + step : step
        let target = min(max(position + step, 0), max(duration, 0))
        position = target
        player?.seek(to: target, mode: .exact)
        skipLabel = (skipTotal < 0 ? "\u{2212}" : "+") + "\(Int(abs(skipTotal))) s"
        skipTask?.cancel()
        skipTask = Task { [weak self] in
            try? await Task.sleep(for: .milliseconds(700))
            guard !Task.isCancelled else { return }
            self?.skipLabel = nil
            self?.skipTotal = 0
        }
    }

    /// Hold on the right half: 2× while held.
    func setFastForward(_ on: Bool) {
        isFastForwarding = on
        player?.setSpeed(on ? 2 : 1)
    }

    func setFill(_ on: Bool) {
        fill = on
        player?.setFill(on)
    }

    func toggleRotationLock() {
        rotationLocked.toggle()
        rotationLocked ? Orientation.lockCurrent() : Orientation.unlock()
        pokeChrome()
    }

    func selectAudio(_ id: String) { player?.setAudioTrack(id) }
    func selectSubtitle(_ id: String) { player?.setSubtitleTrack(id) }

    // MARK: Scrubbing

    func beginScrub(at seconds: Double) {
        scrubTime = clamp(seconds)
        scrubSpeed = .full
        pokeChrome()
    }

    /// Moves the scrub position; sends throttled keyframe seeks so the frame under
    /// the scrim follows, but the preview (from memory) always leads.
    func updateScrub(to seconds: Double, speed: ScrubSpeed = .full) {
        guard scrubTime != nil else { return }
        let t = clamp(seconds)
        if let old = scrubTime, duration > 0, (old > 0 && t == 0) || (old < duration && t == duration) {
            Haptics.edge()
        }
        if speed != scrubSpeed {
            scrubSpeed = speed
            Haptics.selection()
        }
        scrubTime = t
        pendingScrubSeek = t
        flushScrubSeek()
    }

    /// On release: keyframe seek for an instant resume, then a precise seek once
    /// that frame shows. Playing stays playing; paused stays paused.
    func endScrub() {
        guard let t = scrubTime else { return }
        pendingScrubSeek = nil
        position = t
        exactSeekAfterRestart = t
        player?.seek(to: t, mode: .keyframe)
        seekInFlight = true
        lastSeekAt = .now
        scrubTime = nil
        scrubSpeed = .full
        if phase == .ended && t < duration - 0.5 { phase = .playing }
        pokeChrome()
    }

    func cancelScrub() {
        scrubTime = nil
        pendingScrubSeek = nil
        scrubSpeed = .full
    }

    private func flushScrubSeek() {
        guard let t = pendingScrubSeek, !seekInFlight else { return }
        let wait = 0.12 - Date.now.timeIntervalSince(lastSeekAt)
        if wait > 0 {
            Task { [weak self] in
                try? await Task.sleep(for: .seconds(wait))
                self?.flushScrubSeek()
            }
            return
        }
        pendingScrubSeek = nil
        seekInFlight = true
        lastSeekAt = .now
        player?.seek(to: t, mode: .keyframe)
    }

    private func clamp(_ t: Double) -> Double { min(max(t, 0), max(duration, 0)) }

    // MARK: Chrome

    func toggleChrome() {
        chromeVisible ? hideChrome() : pokeChrome()
    }

    /// Shows the chrome and restarts the 3-second hide timer.
    func pokeChrome() {
        if !chromeVisible { withAnimation(Riffle.controlsFade) { chromeVisible = true } }
        hideTask?.cancel()
        hideTask = Task { [weak self] in
            try? await Task.sleep(for: .seconds(Riffle.controlsAutoHideDelay))
            guard !Task.isCancelled, let self, self.canAutoHide else { return }
            self.hideChrome()
        }
    }

    private func hideChrome() {
        hideTask?.cancel()
        withAnimation(Riffle.controlsFade) { chromeVisible = false }
    }

    /// Chrome stays while paused, buffering, scrubbing, with a sheet open, and
    /// whenever VoiceOver or Switch Control runs.
    private var canAutoHide: Bool {
        #if DEBUG
        // `defaults write dev.isilva.riffle debug.keepChrome -bool YES` for screenshots.
        if UserDefaults.standard.bool(forKey: "debug.keepChrome") { return false }
        #endif
        return phase == .playing && !isPaused && !isBuffering && scrubTime == nil && !sheetOpen
            && !UIAccessibility.isVoiceOverRunning && !UIAccessibility.isSwitchControlRunning
    }

    // MARK: Events

    private func handle(_ event: MPVEvent) {
        switch event {
        case .timePos(let t):
            position = t
        case .duration(let d):
            if d > 0 { duration = d }
        case .paused(let p):
            isPaused = p
            if p { pokeChrome() }
        case .pausedForCache(let b):
            setBuffering(b)
        case .seeking:
            break
        case .cacheEnd(let t):
            buffered = t
        case .eofReached(let eof):
            if eof && phase == .playing {
                phase = .ended
                pokeChrome()
                Task { await reportProgress(force: true) }
            }
        case .videoSize(let w, let h):
            displayAspect = Double(w) / Double(h)
            decideOrientation()
        case .audioTrack(let id):
            audioTrack = id
        case .subtitleTrack(let id):
            subtitleTrack = id
        case .fileLoaded:
            break
        case .playbackRestart:
            seekInFlight = false
            if !hasStarted {
                hasStarted = true
                phase = .playing
                pokeChrome()
            }
            setBuffering(false)
            if let exact = exactSeekAfterRestart {
                exactSeekAfterRestart = nil
                player?.seek(to: exact, mode: .exact)
            } else {
                flushScrubSeek()
            }
        case .endFile(let error):
            if let error {
                phase = .failed(title: "Couldn't play this video", message: Self.describe(error))
                bufferTask?.cancel()
                isBuffering = false
            }
        }
    }

    private func setBuffering(_ b: Bool) {
        guard b != isBuffering else { return }
        isBuffering = b
        if b {
            beginBufferWatch(firstOpen: false)
            pokeChrome()
        } else {
            bufferTask?.cancel()
            statusText = nil
            stalled = false
            pokeChrome()
        }
    }

    /// After 2 s on a first open: "Waking up the disk…". After 15 s: give up waiting.
    private func beginBufferWatch(firstOpen: Bool) {
        bufferTask?.cancel()
        bufferTask = Task { [weak self] in
            try? await Task.sleep(for: .seconds(2))
            guard !Task.isCancelled, let self else { return }
            if firstOpen { self.statusText = "Waking up the disk\u{2026}" }
            try? await Task.sleep(for: .seconds(13))
            guard !Task.isCancelled else { return }
            self.stalled = true
        }
    }

    /// Wide videos turn to landscape once, on open, before the first frame. Without a
    /// display size from the server, wait for mpv's.
    private func decideOrientation() {
        guard !orientationDecided, let aspect = displayAspect else { return }
        orientationDecided = true
        let wantsLandscape = UserDefaults.standard.object(forKey: "player.landscapeForWide") as? Bool ?? true
        Orientation.enterPlayer(requestLandscape: wantsLandscape && aspect > 1.1)
    }

    private func reportProgress(force: Bool = false) async {
        guard hasStarted, duration > 0 else { return }
        let pos = phase == .ended ? duration : position
        guard force || (!isPaused && abs(pos - lastReported) >= 1) else { return }
        lastReported = pos
        await model.saveProgress(request.item.id, position: pos, duration: duration)
    }

    private func observeAppState() {
        let center = NotificationCenter.default
        tasks.append(Task { [weak self] in
            for await _ in center.notifications(named: UIApplication.didEnterBackgroundNotification) {
                guard let self else { return }
                self.player?.setPaused(true)
                self.player?.setVideoEnabled(false)
                await self.reportProgress(force: true)
            }
        })
        tasks.append(Task { [weak self] in
            for await _ in center.notifications(named: UIApplication.willEnterForegroundNotification) {
                self?.player?.setVideoEnabled(true)
            }
        })
    }

    private static func describe(_ mpvError: String) -> String {
        let e = mpvError.lowercased()
        if e.contains("unrecognized file format") || e.contains("no audio or video") {
            return "The file uses a format Riffle can't decode."
        }
        if e.contains("loading failed") || e.contains("network") {
            return "Can't reach Riffle Server. Check that it's running."
        }
        return "Something went wrong while playing (\(mpvError)). Try again."
    }
}

@MainActor
enum Haptics {
    private static let light = UIImpactFeedbackGenerator(style: .light)
    private static let select = UISelectionFeedbackGenerator()
    static func edge() { light.impactOccurred() }
    static func selection() { select.selectionChanged() }
}

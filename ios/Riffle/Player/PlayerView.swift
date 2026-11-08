import SwiftUI

/// The full-screen player (`design/player.md`).
struct PlayerView: View {
    @Environment(AppModel.self) private var app
    var request: PlaybackRequest
    var onClose: () -> Void
    var setOverlaysHidden: (Bool) -> Void

    @State private var player: PlayerModel?

    var body: some View {
        ZStack {
            Riffle.Palette.stage.ignoresSafeArea()
            if let player {
                PlayerScreen(player: player, close: close)
                    .onChange(of: player.chromeVisible, initial: true) { _, visible in
                        setOverlaysHidden(!visible)
                    }
            }
        }
        .onAppear {
            guard player == nil else { return }
            let p = PlayerModel(request: request, model: app)
            player = p
            p.start()
        }
        .preferredColorScheme(.dark)
    }

    private func close() {
        Task {
            await player?.stop()
            onClose()
        }
    }
}

private struct PlayerScreen: View {
    @Bindable var player: PlayerModel
    var close: () -> Void

    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @State private var dismissOffset: CGFloat = 0
    @State private var panStart: Double = 0

    var body: some View {
        GeometryReader { geo in
            ZStack {
                MPVVideoView(layer: player.layer, onReady: player.videoLayerReady, onResize: player.videoLayerResized)
                    .ignoresSafeArea()
                GestureLayer(handlers: handlers(width: geo.size.width), dismissEnabled: player.scrubTime == nil)
                    .ignoresSafeArea()
                PlayerControls(player: player, landscape: geo.size.width > geo.size.height, close: close)
            }
            .scaleEffect(reduceMotion ? 1 : 1 - min(dismissOffset / 1600, 0.3))
            .offset(y: reduceMotion ? 0 : dismissOffset)
            .opacity(reduceMotion ? 1 - min(dismissOffset / 400, 0.6) : 1)
        }
        .sheet(isPresented: $player.sheetOpen) {
            TrackSheet(
                probe: player.request.item.probe,
                selectedAudio: player.audioTrack,
                selectedSubtitle: player.subtitleTrack,
                onSelectAudio: player.selectAudio,
                onSelectSubtitle: player.selectSubtitle
            )
        }
        .accessibilityAction(named: player.fill ? "Fit" : "Fill") { player.setFill(!player.fill) }
        .onKeyPress(.space) { player.togglePlay(); return .handled }
        .onKeyPress(.escape) { close(); return .handled }
        .onKeyPress(keys: [.leftArrow, .rightArrow]) { press in
            let step = press.modifiers.contains(.shift) ? 60.0 : 10.0
            player.skip(press.key == .leftArrow ? -step : step)
            return .handled
        }
        .focusable()
        .focusEffectDisabled()
    }

    private func handlers(width: CGFloat) -> GestureLayer.Handlers {
        .init(
            tap: { player.toggleChrome() },
            skip: { forward, continuing in player.tapSkip(forward: forward, continuing: continuing) },
            scrubBegan: {
                panStart = player.position
                player.beginScrub(at: panStart)
            },
            scrubChanged: { fraction in
                // One screen width is 2 minutes, or the whole video if shorter.
                let span = min(120, player.duration)
                player.updateScrub(to: panStart + Double(fraction) * span)
            },
            scrubEnded: { player.endScrub() },
            dismissChanged: { dy in dismissOffset = dy },
            dismissEnded: { dy, velocity in
                if dy > 140 || velocity > 900 {
                    close()
                } else {
                    withAnimation(.spring) { dismissOffset = 0 }
                }
            },
            hold: { active in player.setFastForward(active) },
            pinch: { fill in player.setFill(fill) }
        )
    }
}

/// Title bar, transport and scrubber over `scrim` (`design/components/PlayerControls.md`).
private struct PlayerControls: View {
    var player: PlayerModel
    var landscape: Bool
    var close: () -> Void

    var body: some View {
        ZStack {
            if player.chromeVisible {
                Riffle.Palette.scrim.ignoresSafeArea().allowsHitTesting(false).transition(.opacity)
            }
            centre
            VStack(spacing: 0) {
                if player.chromeVisible { topBar.transition(.opacity) }
                Spacer(minLength: 0)
                if player.chromeVisible && player.phase != .opening && !isFailed {
                    scrubber
                        .padding(.horizontal, landscape ? Riffle.Spacing.s6 : Riffle.Spacing.s4)
                        .padding(.bottom, landscape ? Riffle.Spacing.s4 : Riffle.Spacing.s6)
                        .transition(.opacity)
                }
            }
            overlays
        }
    }

    private var isFailed: Bool {
        if case .failed = player.phase { return true }
        return false
    }

    // MARK: Top

    private var topBar: some View {
        HStack(spacing: Riffle.Spacing.s2) {
            iconButton("xmark", label: "Close", action: close)
            VStack(alignment: .leading, spacing: 2) {
                Text(player.title)
                    .font(Riffle.Typography.headline)
                    .foregroundStyle(Riffle.Palette.ink)
                    .lineLimit(1)
                Text(player.subtitle)
                    .font(Riffle.Typography.caption)
                    .foregroundStyle(Riffle.Palette.ink)
                    .lineLimit(1)
            }
            Spacer(minLength: Riffle.Spacing.s2)
            Button(action: player.toggleRotationLock) {
                Image(systemName: "lock.rotation")
                    .font(Riffle.Typography.headline)
                    .foregroundStyle(player.rotationLocked ? Riffle.Palette.stage : Riffle.Palette.ink)
                    .frame(width: 34, height: 34)
                    .background(player.rotationLocked ? Riffle.Palette.ink : .clear, in: .circle)
                    .frame(width: Riffle.minimumHitTarget, height: Riffle.minimumHitTarget)
                    .contentShape(.rect)
            }
            .buttonStyle(DimOnPress())
            .accessibilityLabel("Rotation lock")
            .accessibilityValue(player.rotationLocked ? "On" : "Off")
            iconButton("text.bubble", label: "Audio and subtitles") { player.sheetOpen = true }
                .disabled(player.request.item.probe == nil)
        }
        .padding(.horizontal, Riffle.Spacing.s2)
    }

    private func iconButton(_ symbol: String, label: String, action: @escaping () -> Void) -> some View {
        Button(action: action) {
            Image(systemName: symbol)
                .font(Riffle.Typography.headline)
                .foregroundStyle(Riffle.Palette.ink)
                .frame(width: Riffle.minimumHitTarget, height: Riffle.minimumHitTarget)
                .contentShape(.rect)
        }
        .buttonStyle(DimOnPress())
        .accessibilityLabel(label)
    }

    // MARK: Centre

    @ViewBuilder private var centre: some View {
        switch player.phase {
        case .failed(let title, let message):
            NoticeView(tone: .alert, title: title, message: message, actionLabel: "Try again", action: player.retry)
                .overlay(alignment: .bottomTrailing) {
                    Button("Close", action: close).buttonStyle(.rifflePlain).padding(Riffle.Spacing.s2)
                }
                .frame(maxWidth: 420)
                .padding(Riffle.Spacing.s4)
        case _ where player.stalled:
            NoticeView(tone: .alert, title: "Still waiting for Riffle Server",
                       message: "The video isn't arriving. Check that Riffle Server is running.",
                       actionLabel: "Try again", action: player.retry)
                .frame(maxWidth: 420)
                .padding(Riffle.Spacing.s4)
        case .opening:
            VStack(spacing: Riffle.Spacing.s4) {
                if player.showSpinner { spinner }
                status
            }
        default:
            if player.chromeVisible && player.scrubTime == nil {
                VStack(spacing: Riffle.Spacing.s4) {
                    HStack(spacing: Riffle.Spacing.s6) {
                        TransportButton(kind: .back) { player.skip(-10) }
                        if player.isBuffering && player.phase != .ended {
                            spinner.frame(width: 72, height: 72)
                        } else {
                            TransportButton(kind: player.phase == .ended ? .replay : (player.isPaused ? .play : .pause),
                                            action: player.togglePlay)
                        }
                        TransportButton(kind: .forward) { player.skip(10) }
                    }
                    status
                }
                .transition(.opacity)
            }
        }
    }

    private var spinner: some View {
        ProgressView().controlSize(.large).tint(Riffle.Palette.ink)
    }

    @ViewBuilder private var status: some View {
        if let text = player.statusText {
            Text(text).font(Riffle.Typography.body).foregroundStyle(Riffle.Palette.ink)
        }
    }

    // MARK: Bottom

    private var scrubber: some View {
        Scrubber(
            duration: player.duration,
            time: player.displayedTime,
            buffered: player.buffered,
            isScrubbing: player.scrubTime != nil,
            scrubLabel: player.scrubSpeed.label,
            previewAspect: player.trickplay.aspect(fallback: player.displayAspect),
            preview: { player.trickplay.image(at: $0) },
            onBegin: { player.beginScrub(at: $0) },
            onChange: { player.updateScrub(to: $0, speed: $1) },
            onEnd: { player.endScrub() },
            onStep: { player.skip($0) }
        )
    }

    // MARK: Transient labels

    @ViewBuilder private var overlays: some View {
        VStack {
            if player.isFastForwarding {
                label("2\u{00D7}").padding(.top, Riffle.Spacing.s6)
            }
            Spacer()
        }
        if let skip = player.skipLabel {
            label(skip)
        }
    }

    private func label(_ text: String) -> some View {
        Text(text)
            .font(Riffle.Typography.headline)
            .monospacedDigit()
            .foregroundStyle(Riffle.Palette.ink)
            .padding(.horizontal, Riffle.Spacing.s4)
            .padding(.vertical, Riffle.Spacing.s2)
            .background(Riffle.Palette.scrim, in: .capsule)
            .allowsHitTesting(false)
    }
}

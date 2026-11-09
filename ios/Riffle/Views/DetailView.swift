import SwiftUI

/// A video's detail screen: still, title, format, and Resume / Play from start.
/// Opening it warms the disk so playback doesn't wait on spin-up.
struct DetailView: View {
    @Environment(AppModel.self) private var model
    var itemID: String
    var fallback: Item
    @Binding var playing: PlaybackRequest?

    private var item: Item { model.item(itemID) ?? fallback }
    private var progress: WatchProgress? { model.progress[itemID] }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: Riffle.Spacing.s6) {
                PosterImage(item: item)
                    .overlay(alignment: .bottom) {
                        if let progress, progress.isInProgress { ProgressBar(fraction: progress.fraction) }
                    }
                    .clipShape(.rect(cornerRadius: Riffle.Radius.md))

                VStack(alignment: .leading, spacing: Riffle.Spacing.s2) {
                    Text(item.displayTitle)
                        .font(Riffle.Typography.title)
                        .foregroundStyle(Riffle.Palette.ink)
                    Text(caption)
                        .font(Riffle.Typography.caption)
                        .foregroundStyle(Riffle.Palette.inkMuted)
                    if let status = watchStatus {
                        HStack(spacing: Riffle.Spacing.s1) {
                            if progress?.state == .watched { WatchedMark() }
                            Text(status)
                        }
                        .font(Riffle.Typography.caption)
                        .foregroundStyle(Riffle.Palette.inkMuted)
                        .accessibilityElement(children: .combine)
                    }
                    BadgeRow(badges: Format.badges(item, limit: 3))
                }

                if let reason = item.probeError {
                    NoticeView(
                        tone: .alert,
                        title: "Couldn't read this file",
                        message: "ffprobe on the server said: \(reason). Riffle may still be able to play it."
                    )
                }

                actions

                if item.probe?.video != nil, item.trickplay == .pending || item.trickplay == .none {
                    NoticeView(
                        tone: .neutral,
                        title: "Making scrub previews",
                        message: "You can play now. Previews appear in the scrubber as soon as they're ready."
                    )
                }

                if let probe = item.probe { TechnicalDetails(item: item, probe: probe) }
            }
            .padding(.horizontal, Riffle.Spacing.s4)
            .padding(.bottom, Riffle.Spacing.s6)
            .frame(maxWidth: 720)
            .frame(maxWidth: .infinity)
        }
        .navigationBarTitleDisplayMode(.inline)
        .riffleScreen()
        .task(id: itemID) { try? await model.client?.warm(itemID) }
        .toolbar {
            ToolbarItem(placement: .topBarTrailing) {
                Menu {
                    if progress?.state == .watched {
                        Button("Mark as unwatched", systemImage: "circle") {
                            Task { await model.setWatched(itemID, false) }
                        }
                    } else {
                        Button("Mark as watched", systemImage: "checkmark.circle") {
                            Task { await model.setWatched(itemID, true) }
                        }
                    }
                } label: {
                    Image(systemName: "ellipsis")
                        .foregroundStyle(Riffle.Palette.ink)
                        .accessibilityLabel("More")
                }
            }
        }
    }

    /// "Watched", "Watched 3 times", "In progress · 23:10 left"; nil when unwatched.
    private var watchStatus: String? {
        guard let progress else { return nil }
        switch progress.state {
        case .watched:
            let times = progress.timesFinished ?? 1
            return times > 1 ? "Watched \(times) times" : "Watched"
        case .partial:
            let left = max(progress.duration - progress.position, 0)
            return left > 0 ? "In progress · \(Format.time(left)) left" : "In progress"
        case .unwatched:
            return nil
        }
    }

    private var caption: String {
        let folder = ([item.root] + item.folderComponents).joined(separator: " / ")
        return [folder, Format.meta(item)].joined(separator: " · ")
    }

    @ViewBuilder private var actions: some View {
        VStack(spacing: Riffle.Spacing.s2 + 4) {
            if let progress, progress.isResumable {
                Button {
                    playing = PlaybackRequest(item: item, start: progress.position)
                } label: {
                    Label("Resume from \(Format.time(progress.position))", systemImage: "play.fill")
                }
                .buttonStyle(.rifflePrimary)
                Button("Play from start") { playing = PlaybackRequest(item: item, start: 0) }
                    .buttonStyle(.riffleSecondary)
            } else {
                Button {
                    playing = PlaybackRequest(item: item, start: 0)
                } label: {
                    Label("Play", systemImage: "play.fill")
                }
                .buttonStyle(.rifflePrimary)
            }
        }
    }
}

/// What's in the file: video, audio and subtitle streams, container and size.
private struct TechnicalDetails: View {
    var item: Item
    var probe: Probe

    var body: some View {
        VStack(alignment: .leading, spacing: Riffle.Spacing.s4) {
            Text("Details").font(Riffle.Typography.title).foregroundStyle(Riffle.Palette.ink)
            VStack(spacing: 0) {
                if let v = probe.video { row("Video", video(v)) }
                ForEach(Array(probe.audio.enumerated()), id: \.offset) { i, a in
                    row(i == 0 ? "Audio" : "", audio(a))
                }
                ForEach(Array(probe.subtitles.enumerated()), id: \.offset) { i, s in
                    row(i == 0 ? "Subtitles" : "", subtitle(s))
                }
                row("File", [item.name, Format.size(item.size)]
                    .compactMap { $0 }.joined(separator: " · "), last: true)
            }
            .background(Riffle.Palette.surfaceRaised, in: .rect(cornerRadius: Riffle.Radius.md))
        }
    }

    private func row(_ label: String, _ value: String, last: Bool = false) -> some View {
        VStack(spacing: 0) {
            HStack(alignment: .firstTextBaseline, spacing: Riffle.Spacing.s4) {
                Text(label)
                    .font(Riffle.Typography.caption)
                    .foregroundStyle(Riffle.Palette.inkMuted)
                    .frame(width: 76, alignment: .leading)
                Text(value)
                    .font(Riffle.Typography.body)
                    .foregroundStyle(Riffle.Palette.ink)
                    .frame(maxWidth: .infinity, alignment: .leading)
            }
            .padding(.horizontal, Riffle.Spacing.s4)
            .padding(.vertical, Riffle.Spacing.s2 + 4)
            if !last {
                Rectangle().fill(Riffle.Palette.line).frame(height: 1).padding(.leading, Riffle.Spacing.s4)
            }
        }
        .accessibilityElement(children: .combine)
    }

    private func video(_ v: VideoStream) -> String {
        var parts = [Format.videoCodec(v.codec)]
        let w = (v.displayWidth ?? 0) > 0 ? v.displayWidth! : v.width
        let h = (v.displayHeight ?? 0) > 0 ? v.displayHeight! : v.height
        parts.append("\(w) × \(h)")
        if let fps = v.frameRate, fps > 0 { parts.append(String(format: "%g fps", (fps * 1000).rounded() / 1000)) }
        return parts.joined(separator: " · ")
    }

    private func audio(_ a: AudioStream) -> String {
        let name = a.title ?? Format.language(a.language) ?? "Track \(a.index)"
        return "\(name) · \(Format.audioCodec(a.codec)) \(Format.channels(a.channels))"
    }

    private func subtitle(_ s: SubStream) -> String {
        let name = s.title ?? Format.language(s.language) ?? "Track \(s.index)"
        return [name, s.codec.uppercased(), s.forced == true ? "forced" : nil].compactMap { $0 }.joined(separator: " · ")
    }
}

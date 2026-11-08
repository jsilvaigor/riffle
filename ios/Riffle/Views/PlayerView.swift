import SwiftUI

/// Placeholder until the mpv-backed player lands (design/player.md).
struct PlayerView: View {
    @Environment(\.dismiss) private var dismiss
    var request: PlaybackRequest

    var body: some View {
        ZStack {
            Riffle.Palette.stage.ignoresSafeArea()
            VStack(spacing: Riffle.Spacing.s4) {
                Text(request.item.displayTitle)
                    .font(Riffle.Typography.headline)
                    .foregroundStyle(Riffle.Palette.ink)
                Text("Starts at \(Format.time(request.start))")
                    .font(Riffle.Typography.timecode)
                    .foregroundStyle(Riffle.Palette.ink)
                Text("The player arrives with MPVKit in the next step.")
                    .font(Riffle.Typography.body)
                    .foregroundStyle(Riffle.Palette.inkMuted)
            }
            .padding(Riffle.Spacing.s6)
        }
        .overlay(alignment: .topLeading) {
            Button { dismiss() } label: {
                Image(systemName: "xmark")
                    .font(Riffle.Typography.headline)
                    .foregroundStyle(Riffle.Palette.ink)
                    .frame(width: Riffle.minimumHitTarget, height: Riffle.minimumHitTarget)
            }
            .accessibilityLabel("Close")
            .padding(Riffle.Spacing.s2)
        }
    }
}

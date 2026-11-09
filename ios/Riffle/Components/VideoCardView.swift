import SwiftUI

/// A 16:9 thumbnail with title and metadata, for shelves (`design/components/VideoCard.md`).
struct VideoCardView: View {
    var item: Item
    var meta: String
    var progress: WatchProgress?

    var body: some View {
        VStack(alignment: .leading, spacing: Riffle.Spacing.s2) {
            PosterImage(item: item)
                .overlay(alignment: .bottomTrailing) {
                    if let d = item.duration, d > 0 {
                        Text(Format.time(d))
                            .font(Riffle.Typography.timecode)
                            .foregroundStyle(Riffle.Palette.ink)
                            .padding(.horizontal, Riffle.Spacing.s1 + 2)
                            .padding(.vertical, 2)
                            .background(Riffle.Palette.scrim, in: .rect(cornerRadius: Riffle.Radius.sm))
                            .padding(Riffle.Spacing.s2)
                            .padding(.bottom, progress == nil ? 0 : 3)
                    }
                }
                .overlay(alignment: .topLeading) {
                    BadgeRow(badges: Format.badges(item, limit: 2), tone: .media)
                        .padding(Riffle.Spacing.s2)
                }
                .overlay(alignment: .bottom) {
                    if let progress { ProgressBar(fraction: progress.fraction) }
                }
                .clipShape(.rect(cornerRadius: Riffle.Radius.md))
            VStack(alignment: .leading, spacing: 2) {
                Text(item.displayTitle)
                    .font(Riffle.Typography.headline)
                    .foregroundStyle(Riffle.Palette.ink)
                    .lineLimit(2, reservesSpace: false)
                    .multilineTextAlignment(.leading)
                Text(meta)
                    .font(Riffle.Typography.caption)
                    .foregroundStyle(Riffle.Palette.inkMuted)
                    .lineLimit(1)
            }
        }
        .accessibilityElement(children: .combine)
    }
}

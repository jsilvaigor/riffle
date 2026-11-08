import SwiftUI

/// A folder or video row in a list (`design/components/LibraryRow.md`).
struct FolderRowView: View {
    var folder: Folder

    var body: some View {
        HStack(spacing: Riffle.Spacing.s4) {
            Image(systemName: "folder")
                .font(Riffle.Typography.body)
                .foregroundStyle(Riffle.Palette.inkMuted)
                .frame(width: 28)
            Text(folder.name)
                .font(Riffle.Typography.body)
                .foregroundStyle(Riffle.Palette.ink)
                .lineLimit(2)
            Spacer(minLength: Riffle.Spacing.s2)
            Text(folder.count, format: .number)
                .font(Riffle.Typography.caption)
                .foregroundStyle(Riffle.Palette.inkMuted)
        }
        .frame(minHeight: Riffle.minimumHitTarget)
        .accessibilityElement(children: .combine)
        .accessibilityLabel("\(folder.name), \(folder.count) videos")
    }
}

struct VideoRowView: View {
    var item: Item
    var progress: WatchProgress?

    var body: some View {
        HStack(spacing: Riffle.Spacing.s4) {
            PosterImage(item: item, cornerRadius: Riffle.Radius.sm)
                .frame(width: 96)
                .overlay(alignment: .bottom) {
                    if let progress, progress.isResumable {
                        ProgressBar(fraction: progress.fraction)
                    }
                }
                .clipShape(.rect(cornerRadius: Riffle.Radius.sm))
            VStack(alignment: .leading, spacing: Riffle.Spacing.s1) {
                Text(item.name)
                    .font(Riffle.Typography.headline)
                    .foregroundStyle(Riffle.Palette.ink)
                    .lineLimit(2)
                if item.probeError != nil {
                    Label("Couldn't read this file", systemImage: "exclamationmark.triangle")
                        .font(Riffle.Typography.caption)
                        .foregroundStyle(Riffle.Palette.alert)
                } else {
                    HStack(spacing: Riffle.Spacing.s2) {
                        Text(Format.meta(item))
                            .font(Riffle.Typography.caption)
                            .foregroundStyle(Riffle.Palette.inkMuted)
                        BadgeRow(badges: Format.badges(item, limit: 3))
                    }
                }
            }
            Spacer(minLength: 0)
        }
        .padding(.vertical, Riffle.Spacing.s1)
        .accessibilityElement(children: .combine)
    }
}

/// The `citron` played-fraction bar along a thumbnail's bottom edge.
struct ProgressBar: View {
    var fraction: Double

    var body: some View {
        GeometryReader { geo in
            ZStack(alignment: .leading) {
                Rectangle().fill(Riffle.Palette.scrim)
                Rectangle().fill(Riffle.Palette.citron).frame(width: geo.size.width * fraction)
            }
        }
        .frame(height: 3)
    }
}

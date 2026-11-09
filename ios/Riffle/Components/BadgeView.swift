import SwiftUI

/// A short uppercase format tag (`design/components/Badge.md`).
struct BadgeView: View {
    enum Tone { case neutral, media, alert }

    var text: String
    var tone: Tone = .neutral

    var body: some View {
        // Badge text arrives already cased ("1080p", "HEVC"), so no uppercasing here.
        Text(text)
            .font(Riffle.Typography.label)
            .tracking(Riffle.Typography.labelTracking)
            .foregroundStyle(tone == .alert ? Riffle.Palette.onCitron : Riffle.Palette.ink)
            .padding(.horizontal, Riffle.Spacing.s1 + 2)
            .padding(.vertical, 2)
            .background(background, in: .rect(cornerRadius: Riffle.Radius.sm))
    }

    private var background: Color {
        switch tone {
        case .neutral: Riffle.Palette.line
        case .media: Riffle.Palette.scrim
        case .alert: Riffle.Palette.alert
        }
    }
}

struct BadgeRow: View {
    var badges: [String]
    var tone: BadgeView.Tone = .neutral

    var body: some View {
        HStack(spacing: Riffle.Spacing.s1) {
            ForEach(badges, id: \.self) { BadgeView(text: $0, tone: tone) }
        }
    }
}

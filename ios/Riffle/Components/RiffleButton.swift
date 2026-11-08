import SwiftUI

/// Button variants from `design/components/Button.md`.
struct RiffleButtonStyle: ButtonStyle {
    enum Variant { case primary, secondary, plain }
    enum Size { case large, regular }

    var variant: Variant
    var size: Size = .regular
    @Environment(\.isEnabled) private var isEnabled

    func makeBody(configuration: Configuration) -> some View {
        let label = configuration.label
            .font(Riffle.Typography.headline)
            .lineLimit(1)

        Group {
            switch variant {
            case .primary:
                label
                    .foregroundStyle(Riffle.Palette.onCitron)
                    .frame(maxWidth: .infinity, minHeight: height)
                    .background(Riffle.Palette.citron, in: .rect(cornerRadius: Riffle.Radius.md))
            case .secondary:
                label
                    .foregroundStyle(Riffle.Palette.ink)
                    .frame(maxWidth: .infinity, minHeight: height)
                    .overlay(
                        RoundedRectangle(cornerRadius: Riffle.Radius.md)
                            .strokeBorder(Riffle.Palette.inkMuted, lineWidth: 1)
                    )
            case .plain:
                label
                    .foregroundStyle(Riffle.Palette.citron)
                    .frame(minWidth: Riffle.minimumHitTarget, minHeight: Riffle.minimumHitTarget)
            }
        }
        .contentShape(.rect)
        .opacity(isEnabled ? (configuration.isPressed ? 0.6 : 1) : 0.35)
    }

    private var height: CGFloat { size == .large ? 50 : 44 }
}

extension ButtonStyle where Self == RiffleButtonStyle {
    static var rifflePrimary: RiffleButtonStyle { .init(variant: .primary, size: .large) }
    static var riffleSecondary: RiffleButtonStyle { .init(variant: .secondary) }
    static var rifflePlain: RiffleButtonStyle { .init(variant: .plain) }
}

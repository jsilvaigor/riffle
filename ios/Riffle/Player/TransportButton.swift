import SwiftUI

/// The round controls at the centre of the player (`design/components/TransportButton.md`).
struct TransportButton: View {
    enum Kind {
        case play, pause, replay, back, forward

        var symbol: String {
            switch self {
            case .play: "play.fill"
            case .pause: "pause.fill"
            case .replay: "arrow.counterclockwise"
            case .back: "gobackward.10"
            case .forward: "goforward.10"
            }
        }

        var label: String {
            switch self {
            case .play: "Play"
            case .pause: "Pause"
            case .replay: "Replay"
            case .back: "Back 10 seconds"
            case .forward: "Forward 10 seconds"
            }
        }

        var isCentre: Bool {
            switch self { case .play, .pause, .replay: true; default: false }
        }
    }

    var kind: Kind
    var action: () -> Void

    var body: some View {
        Button(action: action) {
            if kind.isCentre {
                Image(systemName: kind.symbol)
                    .font(.system(size: 30, weight: .semibold))
                    .foregroundStyle(Riffle.Palette.onCitron)
                    .frame(width: 72, height: 72)
                    .background(Riffle.Palette.citron, in: .circle)
            } else {
                Image(systemName: kind.symbol)
                    .font(.system(size: 28, weight: .regular))
                    .foregroundStyle(Riffle.Palette.ink)
                    .frame(width: 56, height: 56)
                    .contentShape(.circle)
            }
        }
        .buttonStyle(DimOnPress())
        .accessibilityLabel(kind.label)
    }
}

/// Pressed dims to 60%; no colour change (design/README.md, States).
struct DimOnPress: ButtonStyle {
    @Environment(\.isEnabled) private var isEnabled
    func makeBody(configuration: Configuration) -> some View {
        configuration.label.opacity(isEnabled ? (configuration.isPressed ? 0.6 : 1) : 0.35)
    }
}

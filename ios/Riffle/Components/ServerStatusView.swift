import SwiftUI

/// Whether the app can reach Riffle Server, and where (`design/components/ServerStatus.md`).
struct ServerStatusView: View {
    var state: ConnectionState
    var address: String?

    var body: some View {
        HStack(spacing: Riffle.Spacing.s2) {
            icon
            VStack(alignment: .leading, spacing: 2) {
                Text(name).font(Riffle.Typography.headline).foregroundStyle(Riffle.Palette.ink)
                Text(detail).font(Riffle.Typography.caption).foregroundStyle(detailColor)
            }
            Spacer(minLength: 0)
        }
        .accessibilityElement(children: .combine)
    }

    @ViewBuilder private var icon: some View {
        switch state {
        case .connecting:
            ProgressView().tint(Riffle.Palette.ink).frame(width: 24)
        case .connected:
            Image(systemName: "server.rack").foregroundStyle(Riffle.Palette.ink).frame(width: 24)
        case .offline:
            Image(systemName: "exclamationmark.triangle").foregroundStyle(Riffle.Palette.alert).frame(width: 24)
        }
    }

    private var name: String {
        if case .connected(let name, _) = state { return name }
        return "Riffle Server"
    }

    private var stateWord: String {
        switch state {
        case .connecting: "Connecting"
        case .connected: "Connected"
        case .offline: "Offline"
        }
    }

    private var detail: String {
        [stateWord, address.map(Self.shortAddress)].compactMap { $0 }.joined(separator: " · ")
    }

    private var detailColor: Color {
        if case .offline = state { return Riffle.Palette.alert }
        return Riffle.Palette.inkMuted
    }

    /// "http://tower:8770" → "tower:8770".
    static func shortAddress(_ s: String) -> String {
        s.replacingOccurrences(of: "http://", with: "")
    }
}

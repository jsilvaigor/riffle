import SwiftUI

/// An inline message: what happened and what to do (`design/components/Notice.md`).
struct NoticeView: View {
    enum Tone { case alert, neutral }

    var tone: Tone
    var title: String
    var message: String
    var actionLabel: String?
    var action: (() -> Void)?

    var body: some View {
        HStack(alignment: .top, spacing: Riffle.Spacing.s2 + 4) {
            Image(systemName: tone == .alert ? "exclamationmark.triangle" : "info.circle")
                .font(Riffle.Typography.headline)
                .foregroundStyle(tone == .alert ? Riffle.Palette.alert : Riffle.Palette.inkMuted)
                .accessibilityHidden(true)
            VStack(alignment: .leading, spacing: Riffle.Spacing.s1) {
                Text(title).font(Riffle.Typography.headline).foregroundStyle(Riffle.Palette.ink)
                Text(message).font(Riffle.Typography.body).foregroundStyle(Riffle.Palette.inkMuted)
                if let actionLabel, let action {
                    Button(actionLabel, action: action)
                        .buttonStyle(.rifflePlain)
                        .padding(.leading, -2)
                }
            }
            Spacer(minLength: 0)
        }
        .padding(Riffle.Spacing.s4)
        .background(Riffle.Palette.surfaceRaised, in: .rect(cornerRadius: Riffle.Radius.md))
        .accessibilityElement(children: .combine)
    }
}

/// An empty state: one illustration, a title, a message, and an optional action.
struct EmptyStateView<Actions: View>: View {
    var illustration: ImageResource
    var title: String
    var message: String
    @ViewBuilder var actions: Actions

    var body: some View {
        VStack(spacing: Riffle.Spacing.s4) {
            Image(illustration)
                .resizable()
                .scaledToFit()
                .frame(maxWidth: 160)
                .accessibilityHidden(true)
            VStack(spacing: Riffle.Spacing.s2) {
                Text(title).font(Riffle.Typography.title).foregroundStyle(Riffle.Palette.ink)
                Text(message)
                    .font(Riffle.Typography.body)
                    .foregroundStyle(Riffle.Palette.inkMuted)
                    .multilineTextAlignment(.center)
            }
            actions
        }
        .padding(.horizontal, Riffle.Spacing.s6)
        .frame(maxWidth: 420)
    }
}

extension EmptyStateView where Actions == EmptyView {
    init(illustration: ImageResource, title: String, message: String) {
        self.init(illustration: illustration, title: title, message: message) { EmptyView() }
    }
}

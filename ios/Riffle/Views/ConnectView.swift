import SwiftUI

/// First launch, or after "Change server": point the app at Riffle Server.
struct ConnectView: View {
    @Environment(AppModel.self) private var model
    @State private var address = ""
    @State private var token = ""
    @State private var connecting = false
    @State private var error: String?
    @FocusState private var focus: Field?

    private enum Field { case address, token }

    var body: some View {
        ScrollView {
            VStack(spacing: Riffle.Spacing.s6) {
                EmptyStateView(
                    illustration: .emptyServer,
                    title: "Connect to Riffle Server",
                    message: "Enter the address of Riffle Server on your NAS, and its token if you set RIFFLE_TOKEN."
                )
                .padding(.top, 48)

                VStack(spacing: Riffle.Spacing.s2 + 4) {
                    field("Address", text: $address, prompt: "tower:8770")
                        .textContentType(.URL)
                        .keyboardType(.URL)
                        .focused($focus, equals: .address)
                        .submitLabel(.next)
                        .onSubmit { focus = .token }
                    secureField
                }

                if let error {
                    NoticeView(tone: .alert, title: "Couldn't connect", message: error)
                }

                Button {
                    Task { await connect() }
                } label: {
                    if connecting {
                        ProgressView().tint(Riffle.Palette.onCitron)
                    } else {
                        Text("Connect")
                    }
                }
                .buttonStyle(.rifflePrimary)
                .disabled(address.trimmingCharacters(in: .whitespaces).isEmpty || connecting)
            }
            .padding(.horizontal, Riffle.Spacing.s4)
            .frame(maxWidth: 480)
            .frame(maxWidth: .infinity)
        }
        .scrollDismissesKeyboard(.interactively)
        .background(Riffle.Palette.surface)
        .onAppear { if address.isEmpty { focus = .address } }
    }

    private func field(_ label: String, text: Binding<String>, prompt: String) -> some View {
        TextField(label, text: text, prompt: Text(prompt).foregroundStyle(Riffle.Palette.inkMuted))
            .textInputAutocapitalization(.never)
            .autocorrectionDisabled()
            .modifier(FieldChrome(label: label))
    }

    private var secureField: some View {
        SecureField("Token", text: $token, prompt: Text("Token (optional)").foregroundStyle(Riffle.Palette.inkMuted))
            .textContentType(.password)
            .focused($focus, equals: .token)
            .submitLabel(.go)
            .onSubmit { Task { await connect() } }
            .modifier(FieldChrome(label: "Token"))
    }

    private func connect() async {
        guard !connecting else { return }
        focus = nil
        connecting = true
        defer { connecting = false }
        do {
            try await model.connect(address: address, token: token)
            error = nil
        } catch {
            self.error = error.localizedDescription
        }
    }
}

private struct FieldChrome: ViewModifier {
    var label: String

    func body(content: Content) -> some View {
        content
            .font(Riffle.Typography.body)
            .foregroundStyle(Riffle.Palette.ink)
            .padding(.horizontal, Riffle.Spacing.s4)
            .frame(minHeight: 50)
            .background(Riffle.Palette.surfaceRaised, in: .rect(cornerRadius: Riffle.Radius.md))
            .accessibilityLabel(label)
    }
}

import SwiftUI

struct SettingsView: View {
    @Environment(AppModel.self) private var model
    @AppStorage("player.landscapeForWide") private var landscapeForWide = true
    @State private var confirmChange = false
    @State private var rescanMessage: String?

    var body: some View {
        NavigationStack {
            List {
                Section {
                    ServerStatusView(state: model.connection, address: model.serverAddress)
                        .padding(.vertical, Riffle.Spacing.s1)
                    if let count = model.library?.items.count {
                        LabeledContent("Videos", value: count.formatted())
                    }
                    if case .connected(_, let version) = model.connection {
                        LabeledContent("Server version", value: version)
                    }
                    Button("Rescan library") {
                        Task {
                            do {
                                try await model.rescan()
                                rescanMessage = "Rescanning. New videos show up in a minute or two."
                            } catch {
                                rescanMessage = error.localizedDescription
                            }
                        }
                    }
                    Button("Change server") { confirmChange = true }
                } header: {
                    Text("Riffle Server")
                } footer: {
                    if let rescanMessage { Text(rescanMessage) }
                }

                Section {
                    Toggle("Turn to landscape for wide videos", isOn: $landscapeForWide)
                } header: {
                    Text("Player")
                } footer: {
                    Text("When off, the player never rotates by itself.")
                }

                Section {
                    Image(.mark)
                        .resizable()
                        .scaledToFit()
                        .frame(width: 48, height: 48)
                        .frame(maxWidth: .infinity)
                        .listRowBackground(Color.clear)
                        .accessibilityHidden(true)
                    LabeledContent("Version", value: Self.appVersion)
                }
            }
            .listRowBackground(Riffle.Palette.surfaceRaised)
            .navigationTitle("Settings")
            .riffleScreen()
            .confirmationDialog("Change server?", isPresented: $confirmChange, titleVisibility: .visible) {
                Button("Change server", role: .destructive) { model.disconnect() }
            } message: {
                Text("You'll need the address and token again. Resume positions stay on the server.")
            }
        }
    }

    private static var appVersion: String {
        let info = Bundle.main.infoDictionary
        let v = info?["CFBundleShortVersionString"] as? String ?? "?"
        let b = info?["CFBundleVersion"] as? String ?? "?"
        return "\(v) (\(b))"
    }
}

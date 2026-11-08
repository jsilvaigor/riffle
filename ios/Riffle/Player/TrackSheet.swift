import SwiftUI

/// Audio and subtitle choice (`design/components/TrackSheet.md`). mpv numbers each
/// kind of track from 1 in file order, which is the order ffprobe reports them.
struct TrackSheet: View {
    var probe: Probe?
    var selectedAudio: String
    var selectedSubtitle: String
    var onSelectAudio: (String) -> Void
    var onSelectSubtitle: (String) -> Void
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        NavigationStack {
            List {
                Section {
                    ForEach(Array((probe?.audio ?? []).enumerated()), id: \.offset) { i, a in
                        row(name(title: a.title, language: a.language, index: i),
                            detail: "\(Format.audioCodec(a.codec)) \(Format.channels(a.channels))",
                            selected: isSelected(selectedAudio, i, isDefault: a.default == true)) {
                            onSelectAudio("\(i + 1)")
                        }
                    }
                } header: {
                    header("Audio")
                }
                Section {
                    row("Off", detail: nil, selected: selectedSubtitle == "no" || selectedSubtitle.isEmpty) {
                        onSelectSubtitle("no")
                    }
                    ForEach(Array((probe?.subtitles ?? []).enumerated()), id: \.offset) { i, s in
                        row(name(title: s.title, language: s.language, index: i),
                            detail: [s.codec.uppercased(), s.forced == true ? "forced" : nil].compactMap { $0 }.joined(separator: " · "),
                            selected: selectedSubtitle == "\(i + 1)") {
                            onSelectSubtitle("\(i + 1)")
                        }
                    }
                } header: {
                    header("Subtitles")
                }
            }
            .listStyle(.plain)
            .scrollContentBackground(.hidden)
            .background(Riffle.Palette.surfaceRaised)
            .navigationTitle("Audio and subtitles")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .confirmationAction) {
                    Button("Done") { dismiss() }.buttonStyle(.rifflePlain)
                }
            }
        }
        .presentationDetents([.medium, .large])
        .presentationBackground(Riffle.Palette.surfaceRaised)
        .preferredColorScheme(.dark)
    }

    private func isSelected(_ current: String, _ i: Int, isDefault: Bool) -> Bool {
        current == "\(i + 1)" || ((current == "auto" || current.isEmpty) && i == 0)
    }

    private func name(title: String?, language: String?, index: Int) -> String {
        if let title, !title.isEmpty { return title }
        return Format.language(language) ?? "Track \(index + 1)"
    }

    private func header(_ text: String) -> some View {
        Text(text).font(Riffle.Typography.caption).foregroundStyle(Riffle.Palette.inkMuted).textCase(nil)
    }

    private func row(_ title: String, detail: String?, selected: Bool, action: @escaping () -> Void) -> some View {
        Button(action: action) {
            HStack {
                VStack(alignment: .leading, spacing: 2) {
                    Text(title)
                        .font(selected ? Riffle.Typography.headline : Riffle.Typography.body)
                        .foregroundStyle(Riffle.Palette.ink)
                    if let detail {
                        Text(detail).font(Riffle.Typography.caption).foregroundStyle(Riffle.Palette.inkMuted)
                    }
                }
                Spacer()
                if selected {
                    Image(systemName: "checkmark")
                        .font(Riffle.Typography.headline)
                        .foregroundStyle(Riffle.Palette.ink)
                }
            }
            .frame(minHeight: Riffle.minimumHitTarget)
            .contentShape(.rect)
        }
        .buttonStyle(.plain)
        .listRowBackground(Riffle.Palette.surfaceRaised)
        .listRowSeparatorTint(Riffle.Palette.line)
        .accessibilityAddTraits(selected ? .isSelected : [])
    }
}

import SwiftUI

/// A 16:9 still for a video: the server's poster, a `surface-raised` placeholder while
/// it loads (no spinner), or the film glyph when there won't be one. Always cropped.
struct PosterImage: View {
    @Environment(AppModel.self) private var model
    var item: Item
    var cornerRadius: CGFloat = Riffle.Radius.md

    @State private var image: UIImage?
    @State private var unavailable = false

    var body: some View {
        Rectangle()
            .fill(Riffle.Palette.surfaceRaised)
            .aspectRatio(16 / 9, contentMode: .fit)
            .overlay {
                if let image {
                    Image(uiImage: image).resizable().scaledToFill()
                } else if unavailable {
                    Image(systemName: "film")
                        .font(.title2)
                        .foregroundStyle(Riffle.Palette.inkMuted)
                }
            }
            .clipShape(.rect(cornerRadius: cornerRadius))
            .task(id: item.id) { await load() }
            .accessibilityHidden(true)
    }

    private func load() async {
        image = nil
        unavailable = false
        if let cached = await PosterCache.shared.cached(item.id) {
            image = cached
            return
        }
        guard item.probe?.video != nil, item.trickplay != .failed, let client = model.client else {
            unavailable = true
            return
        }
        // Stills come from trickplay, which may still be queued: keep polling gently.
        for _ in 0..<20 {
            guard !Task.isCancelled else { return }
            switch try? await PosterCache.shared.load(item.id, client: client) {
            case .ready(let img):
                withAnimation(.easeOut(duration: 0.2)) { image = img }
                return
            case .pending(let retry):
                try? await Task.sleep(for: .seconds(min(max(retry, 5), 30)))
            case .unavailable, nil:
                unavailable = true
                return
            }
        }
        unavailable = true
    }
}

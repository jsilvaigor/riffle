import CoreGraphics
import Foundation
import ImageIO
import Observation

/// Scrub previews for one video: the manifest plus every sheet, decoded into memory
/// when the player opens so dragging never touches the network (player.md, Scrubbing).
@MainActor
@Observable
final class TrickplayStore {
    enum State: Equatable { case loading, generating, ready, unavailable }

    private(set) var state: State = .loading
    private(set) var manifest: TrickplayManifest?
    private var sheets: [Int: CGImage] = [:]
    @ObservationIgnored private var lastCell: (index: Int, image: CGImage)?

    /// Preview shape before any cell arrives: tile size, else the video's aspect.
    func aspect(fallback: Double?) -> Double {
        manifest?.previewAspect ?? fallback ?? 16 / 9
    }

    /// The thumbnail nearest to `seconds`, or nil while its sheet is still loading.
    func image(at seconds: Double) -> CGImage? {
        guard let m = manifest, let cell = m.cell(at: seconds) else { return nil }
        if let last = lastCell, last.index == cell.index { return last.image }
        guard let sheet = sheets[cell.sheet] else { return nil }
        // Cell rects are in sheet pixels; the last sheet may be shorter than a full grid.
        let rect = cell.rect.intersection(CGRect(x: 0, y: 0, width: sheet.width, height: sheet.height))
        guard !rect.isEmpty, let img = sheet.cropping(to: rect) else { return nil }
        lastCell = (cell.index, img)
        return img
    }

    /// `onManifest` runs as soon as the manifest arrives, before the sheets.
    func load(itemID: String, client: RiffleClient, onManifest: (TrickplayManifest) -> Void = { _ in }) async {
        // 202 means the server moved this item up its queue; retry as it says.
        while !Task.isCancelled {
            do {
                switch try await client.trickplayManifest(itemID) {
                case .ready(let m):
                    manifest = m
                    onManifest(m)
                    await loadSheets(itemID: itemID, manifest: m, client: client)
                    return
                case .pending(let retry):
                    state = .generating
                    try await Task.sleep(for: .seconds(min(max(retry, 2), 15)))
                case .unavailable:
                    state = .unavailable
                    return
                }
            } catch is CancellationError {
                return
            } catch {
                state = .unavailable
                return
            }
        }
    }

    private func loadSheets(itemID: String, manifest m: TrickplayManifest, client: RiffleClient) async {
        // Sheets in time order, four at a time; each decodes off the main actor.
        await withTaskGroup(of: (Int, CGImage?).self) { group in
            var next = 0
            func add() {
                guard next < m.sheets else { return }
                let n = next
                next += 1
                group.addTask {
                    guard let data = try? await client.trickplaySheet(itemID, index: n, format: m.format) else { return (n, nil) }
                    return (n, Self.decode(data))
                }
            }
            for _ in 0..<4 { add() }
            for await (n, image) in group {
                if let image { sheets[n] = image }
                add()
            }
        }
        state = sheets.isEmpty ? .unavailable : .ready
    }

    /// Decodes now rather than lazily on first draw, so the first drag doesn't hitch.
    nonisolated private static func decode(_ data: Data) -> CGImage? {
        guard let src = CGImageSourceCreateWithData(data as CFData, nil) else { return nil }
        let opts = [kCGImageSourceShouldCacheImmediately: true] as CFDictionary
        return CGImageSourceCreateImageAtIndex(src, 0, opts)
    }
}

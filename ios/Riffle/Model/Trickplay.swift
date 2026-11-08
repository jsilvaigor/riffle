import CoreGraphics
import Foundation

extension TrickplayManifest {
    /// Where the thumbnail for a time lives.
    struct Cell: Equatable, Sendable {
        var index: Int
        var sheet: Int
        /// The tile's rectangle in sheet pixels.
        var rect: CGRect
    }

    /// The thumbnail nearest to `seconds`: thumbnail `k` shows `k * interval`, in sheet
    /// `k / (columns*rows)` at cell `k % (columns*rows)`, left to right, top to bottom.
    func cell(at seconds: Double) -> Cell? {
        guard count > 0, interval > 0, columns > 0, rows > 0 else { return nil }
        let k = min(max(Int((seconds / interval).rounded()), 0), count - 1)
        let perSheet = columns * rows
        let local = k % perSheet
        let rect = CGRect(
            x: (local % columns) * tileWidth,
            y: (local / columns) * tileHeight,
            width: tileWidth,
            height: tileHeight
        )
        return Cell(index: k, sheet: k / perSheet, rect: rect)
    }
}

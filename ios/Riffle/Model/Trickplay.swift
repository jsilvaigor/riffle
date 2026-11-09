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
        var rect = CGRect(
            x: (local % columns) * tileWidth,
            y: (local / columns) * tileHeight,
            width: tileWidth,
            height: tileHeight
        )
        // Leave baked-in black bands off the preview, as the player does.
        if let c = crop, c.frameWidth > 0, c.frameHeight > 0 {
            let sx = CGFloat(tileWidth) / CGFloat(c.frameWidth)
            let sy = CGFloat(tileHeight) / CGFloat(c.frameHeight)
            // Round inwards so no sliver of band is left at the edges.
            let x0 = (rect.minX + CGFloat(c.x) * sx).rounded(.up)
            let y0 = (rect.minY + CGFloat(c.y) * sy).rounded(.up)
            let x1 = (rect.minX + CGFloat(c.x + c.w) * sx).rounded(.down)
            let y1 = (rect.minY + CGFloat(c.y + c.h) * sy).rounded(.down)
            rect = CGRect(x: x0, y: y0, width: x1 - x0, height: y1 - y0)
        }
        return Cell(index: k, sheet: k / perSheet, rect: rect)
    }

    /// Width ÷ height of a preview: the tile, minus any baked-in bands.
    var previewAspect: Double? {
        guard tileWidth > 0, tileHeight > 0 else { return nil }
        guard let c = crop, c.frameWidth > 0, c.frameHeight > 0, c.h > 0 else {
            return Double(tileWidth) / Double(tileHeight)
        }
        let w = Double(tileWidth) * Double(c.w) / Double(c.frameWidth)
        let h = Double(tileHeight) * Double(c.h) / Double(c.frameHeight)
        return w / h
    }
}

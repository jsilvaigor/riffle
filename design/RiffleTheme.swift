import SwiftUI

/// Riffle design tokens for SwiftUI.
///
/// Mirrors `design/tokens.json`; keep the two in sync. Usage rules are in
/// `design/README.md`. Riffle is dark-only for now, so set
/// `.preferredColorScheme(.dark)` on the root view.
enum Riffle {

    // MARK: - Color

    enum Palette {
        /// Behind the video itself: letterbox bars, full-screen player backdrop.
        static let stage = Color(hex: 0x000000)
        /// App background for Library, Browse, Search and Settings.
        static let surface = Color(hex: 0x111113)
        /// Cards, sheets, the mini-player bar, menus, loading thumbnails.
        static let surfaceRaised = Color(hex: 0x1C1C1F)
        /// Decorative hairlines and the unplayed scrubber track. Not for control borders.
        static let line = Color(hex: 0x2C2C31)
        /// Primary text and icons on every surface and on `scrim`.
        static let ink = Color(hex: 0xF5F5F0)
        /// Secondary text, the buffered scrubber range, outlined-control borders.
        static let inkMuted = Color(hex: 0x9D9DA3)
        /// The one accent: played scrubber range and thumb, Play button, selected tab, focus, links.
        static let citron = Color(hex: 0xD6F24B)
        /// Text and icons on a `citron` or `alert` fill.
        static let onCitron = surface
        /// Over the video whenever player controls show.
        static let scrim = Color.black.opacity(0.6)
        /// Errors, failed downloads, LIVE badge. Always pair with a word or icon.
        static let alert = Color(hex: 0xFF6B57)
    }

    // MARK: - Type
    //
    // Mapped to iOS text styles so Dynamic Type scales them; the sizes in
    // tokens.json are the default (Large) step of each style.

    enum Typography {
        /// 34/41 bold — the large navigation title, once per screen.
        static let display = Font.largeTitle.weight(.bold)
        /// 22/28 semibold — section headers, title on the detail page.
        static let title = Font.title2.weight(.semibold)
        /// 17/22 semibold — titles on cards and rows, sheet titles, buttons.
        static let headline = Font.headline
        /// 17/22 regular — descriptions, settings, dialogs.
        static let body = Font.body
        /// 13/18 regular — metadata under a title, in `inkMuted`.
        static let caption = Font.footnote
        /// 11/13 semibold, uppercase, tracked — badges only (LIVE, 4K, HDR, CC).
        static let label = Font.caption2.weight(.semibold)
        /// Tracking for `label`: 0.06 em at 11 pt.
        static let labelTracking: CGFloat = 0.66
        /// 10/12 medium — tab bar labels. `TabView` draws its own; use only for a custom bar.
        static let tab = Font.system(size: 10, weight: .medium)
        /// 13/16 medium monospaced — every time readout.
        static let timecode = Font.system(.footnote, design: .monospaced).weight(.medium)
    }

    // MARK: - Spacing

    enum Spacing {
        /// Inside badges; between an icon and its label.
        static let s1: CGFloat = 4
        /// Title to caption; dense thumbnail grids; scrubber hit-area padding.
        static let s2: CGFloat = 8
        /// Screen side margins, card padding, gap between list rows.
        static let s4: CGFloat = 16
        /// Between sections; around the player's transport controls.
        static let s6: CGFloat = 24
    }

    // MARK: - Radius

    enum Radius {
        /// Badges, duration chips, small list thumbnails.
        static let sm: CGFloat = 6
        /// Thumbnails, cards, sheets, the mini-player bar.
        static let md: CGFloat = 12
        // Pill shapes (Play button, scrubber track and thumb, chips): use `Capsule()`.
    }

    // MARK: - Layout

    /// Minimum hit area for anything tappable.
    static let minimumHitTarget: CGFloat = 44
    /// Visible height of the scrubber track (its hit area is `minimumHitTarget`).
    static let scrubberTrackHeight: CGFloat = 4
    /// How long player chrome stays visible after the last interaction.
    static let controlsAutoHideDelay: TimeInterval = 3
    /// Fade duration for player chrome.
    static let controlsFade: Animation = .easeInOut(duration: 0.2)
}

extension Color {
    /// A colour from a 0xRRGGBB literal.
    init(hex: UInt32, opacity: Double = 1) {
        self.init(
            .sRGB,
            red: Double((hex >> 16) & 0xFF) / 255,
            green: Double((hex >> 8) & 0xFF) / 255,
            blue: Double(hex & 0xFF) / 255,
            opacity: opacity
        )
    }
}

extension View {
    /// Styles a short uppercase badge such as LIVE or 4K.
    func riffleLabel() -> some View {
        self
            .font(Riffle.Typography.label)
            .tracking(Riffle.Typography.labelTracking)
            .textCase(.uppercase)
    }
}

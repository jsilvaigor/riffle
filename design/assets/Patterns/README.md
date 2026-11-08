# Patterns

The riffle interleave from the cover as a seamless 96 × 96 tile: pills 8 px tall at a 16 px pitch, alternately from the left and right stacks.

| File | Inks | Use |
| --- | --- | --- |
| `riffle-tile-dark.svg` | `surface-raised` on `surface` | A quiet texture behind onboarding or the server settings header. Never behind body text. |
| `riffle-tile-citron.svg` | `surface` on `citron` | Marketing only: App Store screenshot backgrounds, the repo's social card. Never in the app UI. |

Tile with `background-repeat` (web) or `Image(...).resizable(resizingMode: .tile)` (SwiftUI). Don't scale it: the pitch matches `space-4`.

# TransportButton

The round Play/Pause and skip controls at the centre of the player.

## Kinds
- **play / pause / replay**: a 72 pt `citron` disc with the glyph in `on-citron`. Always the centre control; replay takes its place when the video has ended.
- **back / forward**: a 56 pt hit area, glyph in `ink`, no fill. They skip 10 seconds and flank the centre control `space-6` apart.

## Provide
- `kind`: `play`, `pause`, `back` or `forward`. The accessible label comes with it ("Play", "Back 10 seconds").
- `onClick`.

## Rules
- Only over `scrim` in the player, never on library screens.
- Swap play and pause in place; don't animate the disc's size.
- While buffering, the player replaces the centre disc with a spinner of the same size, so the skip buttons don't move.

**In the app:** SF Symbols `play.fill`, `pause.fill`, `arrow.counterclockwise`, `gobackward.10`, `goforward.10`.

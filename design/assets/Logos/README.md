# Logos

The Riffle mark is a play triangle sliced into five bands, with the second and fourth slid back like cards being riffled: playback and fast scrubbing in one shape. The wordmark is lowercase and monoline, drawn from the same round strokes as the scrubber. The two f's share one crossbar, and the dot of the i is the citron scrubber thumb.

| File | Inks | Use on |
| --- | --- | --- |
| `riffle-mark.svg` | `citron` | `surface`, `surface-raised`, `stage`. The default. |
| `riffle-mark-ink.svg` | `ink` | Dark grounds where something else on screen is already `citron`. |
| `riffle-mark-dark.svg` | `surface` | `citron` or light grounds. |
| `riffle-wordmark.svg` | `ink`, i-dot `citron` | Dark grounds. The default. |
| `riffle-wordmark-ink.svg` | `ink` | Dark grounds, one colour. |
| `riffle-wordmark-dark.svg` | `surface` | `citron` or light grounds. |
| `riffle-lockup.svg` | mark `citron`, word `ink`, dot `citron` | Dark grounds: About screen, README headers, server web page. |
| `riffle-lockup-dark.svg` | `surface` | `citron` or light grounds. |

SVGs show through `<img>`, so they can't inherit colour: pick the file whose ink suits the ground.

- Clear space: half the mark's height on every side.
- Minimum size: the mark at 16 pt, the wordmark at 64 pt wide, the lockup at 96 pt wide.
- Keep the mark pointing right. Don't rotate, mirror or outline it, or change the number of bands.
- Don't retype the wordmark in SF Pro or any other font; always use the files.

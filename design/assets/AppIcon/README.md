# App icon

Full-bleed 1024 × 1024 artwork; iOS applies the rounded mask, so the corners stay square here. The PNGs go straight into an Xcode `AppIcon` set as single-size images.

| File | What | Notes |
| --- | --- | --- |
| `app-icon.png` / `.svg` | Default (light) appearance | `surface` mark on a `citron` field. Opaque, as the App Store requires. |
| `app-icon-dark.png` / `.svg` | Dark appearance | `citron` mark on transparency; iOS supplies the dark backdrop. |
| `app-icon-tinted.png` / `.svg` | Tinted appearance | White mark on black; iOS recolours it with the person's tint. |
| `launch-screen.svg` | Launch screen reference | 1179 × 2556 (iPhone Pro at 3x): the `citron` mark at about 115 pt, centred on `surface`. Build it in the launch storyboard or Info.plist with a `surface` background and the mark as an image, not as this bitmap. |

The mark fills about half the canvas and sits 8 px right of centre, so the triangle looks centred.

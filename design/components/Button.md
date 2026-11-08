# Button

Text buttons for actions outside the player: Resume, Play from start, Rescan library, Try again.

## Variants
- **primary**: `citron` fill, `on-citron` label. The one thing the screen is for (Resume or Play on a video's detail screen). One per view.
- **secondary**: outlined in `ink-muted`, `ink` label. The alternative beside a primary ("Play from start").
- **plain**: `citron` text, no container. Actions inside sheets, notices and navigation bars ("Done", "Try again", "Rescan library").

## Provide
- `children`: a verb in sentence case ("Resume from 1:04:37", "Remove from Continue watching"). Never "OK" or "Submit".
- `size`: `large` (50 pt) for the detail screen's main action; `regular` (44 pt) everywhere else.
- `icon`: optional leading glyph, e.g. `play` for Resume.
- `onClick`, `disabled`.

## Rules
- Corners are `radius-md`; labels are `headline`.
- Pressed dims to 60%; disabled is 35% and stays visible.
- Don't put a primary button in the player: transport controls do that job.

**In the app:** a SwiftUI `ButtonStyle` per variant; `play.fill` for the icon.

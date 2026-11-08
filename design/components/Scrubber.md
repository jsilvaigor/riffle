# Scrubber

Riffle's signature control: the timeline you drag to seek, with an instant trickplay preview above your finger.

## Provide
- `duration`, `position` and `buffered`, all in seconds (`buffered` is the furthest point loaded, not a length).
- `getPreview(seconds)`: returns the frame for that time. Use `Riffle.trickplayCell(manifest, seconds, sheetUrl)` with the Riffle Server trickplay manifest; it returns the sprite cell, which the scrubber scales into its 160 × 90 frame. Return `null` while the server is still generating sheets: the frame then reads "Preview not ready" and the time still shows.
- `onScrub(seconds)` while dragging, `onSeek(seconds)` on release.

## Anatomy
- Track: `radius-pill`, 4 pt (8 pt while dragging). Unplayed `line`, buffered `ink-muted`, played `citron`.
- Thumb: 14 pt `citron` disc, scales to 1.6× while dragging.
- Preview: the frame in a 2 pt `ink` border with `radius-md`, the target time under it in `timecode` on `scrim`.
- Readouts: elapsed left, remaining right with a minus sign, in `timecode` and `ink`.

## Rules
- The hit area is the full 44 pt height even though the track is 4 pt.
- While dragging, show only the sprite frame. Never wait on the network: the app prefetches every sheet when the video opens.
- On release, seek to the nearest keyframe first, then precisely (see docs/architecture.md).
- Arrow keys step 10 seconds (60 with Shift); Home and End jump to the ends. VoiceOver reads "1:04:37 of 1:52:10".

**In the app:** a custom SwiftUI view driven by mpv's time-pos. Sprite math must match the server's trickplay contract: tile k is at sheet k / 100, cell k % 100, left to right then top to bottom.

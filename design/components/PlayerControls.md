# PlayerControls

The overlay shown over a playing video: title bar, transport controls and scrubber, on `scrim`.

## Provide
- `title` (the file or show name) and `subtitle` (folder and format, e.g. "Movies · HEVC 4K · AC3 5.1").
- `state`: `playing`, `paused` or `buffering`. With `buffering`, pass `statusText`, e.g. "Waking up the disk…" while the NAS spins up an array disk.
- Scrubber props: `duration`, `position`, `buffered`, `getPreview`, `onSeek`, `onScrub`.
- Handlers: `onTogglePlay`, `onBack`, `onForward`, `onClose`, `onTracks` (opens TrackSheet), `onPip`.

## Layout
- Top: Close at the left, titles, then Audio and subtitles and Picture in Picture at the right. `space-4` from the edges.
- Centre: back, Play/Pause, forward, `space-6` apart.
- Bottom: the scrubber, `space-6` from the sides and `space-4` from the bottom, inside the safe area.

## Rules
- The whole overlay fades in and out over 200 ms. It hides 3 seconds after the last touch while playing, and stays while paused, buffering or scrubbing.
- Everything on `scrim` is `ink` or `citron`: never `ink-muted`, which drops below 3:1 over a bright frame.
- The video is never rounded or inset; letterbox bars are `stage`.

**In the app:** a SwiftUI overlay above the mpv render view.

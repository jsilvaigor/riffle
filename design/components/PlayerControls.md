# PlayerControls

The overlay shown over a playing video: title bar, transport controls and scrubber, on `scrim`.

## Provide
- `title` (the file or show name) and `subtitle` (folder and format, e.g. "Movies · HEVC 4K · AC3 5.1").
- `state`: `playing`, `paused` or `buffering`. With `buffering`, pass `statusText`, e.g. "Waking up the disk…" while the NAS spins up an array disk.
- Scrubber props: `duration`, `position`, `buffered`, `getPreview`, `onSeek`, `onScrub`.
- Handlers: `onTogglePlay`, `onBack`, `onForward`, `onClose`, `onTracks` (opens TrackSheet), `onPip`, `onRotationLock`.
- Full screen: `fill`, `orientation` (`portrait` or `landscape`, the way the phone is held), `safeArea` insets, `videoGravity` (`fit` or `fill`), `previewAspect` (the video's display aspect).
- `scrubTime` and `scrubLabel` while scrubbing: the transport controls hide so the frame and preview stay clear.
- `chrome: false` for the video playing untouched.

## Layout
- Top: Close at the left, titles, then Rotation lock, Audio and subtitles and Picture in Picture at the right, inside the safe area.
- Rotation lock is `ink` when off and an `ink` disc with a dark glyph when on. It pins the current orientation; see [player.md](../player.md) for when the player rotates by itself.
- Centre: back, Play/Pause, forward, `space-6` apart.
- Bottom: the scrubber, inside the safe area: `space-6` from the sides and `space-4` from the bottom in landscape, `space-4` and `space-6` in portrait (clear of the home indicator).
- Portrait videos play in portrait. On a phone held upright, a 9:16 video fills the width and the chrome sits in the bars above and below it; a landscape video sits in the middle with `stage` bars until the phone turns.

## Rules
- The whole overlay fades in and out over 200 ms. It hides 3 seconds after the last touch while playing, and stays while paused, buffering or scrubbing.
- Everything on `scrim` is `ink` or `citron`: never `ink-muted`, which drops below 3:1 over a bright frame.
- The video is never rounded or inset; letterbox bars are `stage`.

**In the app:** a SwiftUI overlay above the mpv render view.

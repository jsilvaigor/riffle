# Badge

A short uppercase tag for format facts: resolution, HDR, codec, channels, captions.

## Provide
- `children`: one word or number from ffprobe: 4K, 1080p, HDR, HEVC, H.264, WMV, 5.1, CC.
- `tone`: `neutral` on `surface` (`line` fill, `ink` text); `media` over a thumbnail or the video (`scrim` fill); `alert` for a failure ("Failed"), with `on-citron` text.

## Rules
- `label` type, uppercase, `radius-sm`.
- At most two on a thumbnail, three in a row's metadata. Pick what helps choose: resolution and HDR first, codec only when it's unusual for the library.
- Never a sentence, never a status that changes while you watch.

# Illustrations

Flat empty-state pictures built from the system's own shapes: thumbnails, server slabs, the scrubber. 240 × 160, drawn in the token colours as fixed hex values, for `surface` grounds only.

| File | Screen | Pair with |
| --- | --- | --- |
| `empty-library.svg` | Library with no videos yet | "No videos yet" + "Add folders to RIFFLE_MEDIA on the server, then rescan." |
| `empty-server.svg` | First launch, no server set | "Connect to Riffle Server" + a primary Button. |
| `offline.svg` | Server unreachable | "Can't reach Riffle Server" + "Try again". Uses `alert`. |
| `previews-generating.svg` | Scrub previews still being made | "Making scrub previews". The citron tile is the one in progress. |

- Show at most 160 pt wide, centred, `space-4` above the title (`title` style), with the message in `body` and `ink-muted`.
- One illustration per screen, and never in the player.
- Each holds a single `citron` element, so the screen's primary button should be the only other `citron` thing nearby.

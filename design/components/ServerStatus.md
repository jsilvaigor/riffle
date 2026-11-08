# ServerStatus

A compact pill that shows whether the app can reach Riffle Server, and where.

## Provide
- `state`: `connected`, `connecting` or `offline`.
- `name` (from `/api/health`, default "Riffle Server") and `address` (LAN address or Tailscale name).

## Rules
- Connected: server glyph in `ink`, state in `ink-muted`. Connecting: a small spinner. Offline: alert glyph, state in `alert`.
- Every state has a word, so colour never carries it alone.
- Show it in Settings and at the top of Library when not connected; don't show it in the player.

**In the app:** SF Symbols `server.rack` and `exclamationmark.triangle`.

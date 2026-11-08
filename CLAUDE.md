# Riffle

Personal iOS video player that streams from the owner's Unraid NAS. **Scrubbing
performance is the top priority.** Read `docs/architecture.md` for decisions and status.

## Layout

- `server/`: Riffle Server (Go 1.24, standard library only, shells out to ffmpeg/ffprobe).
  Runs in Docker on Unraid. API documented in `server/README.md`.
- `ios/`: Riffle iOS app (not started yet). Plan: SwiftUI + MPVKit (libmpv).
- `design/`: design system. `tokens.json` is the source; `RiffleTheme.swift` mirrors it
  for SwiftUI (change both together); `README.md` has the usage rules the app follows;
  `components/` has a guideline per app control. Build iOS UI from these.
- `docs/`: architecture and decisions.

## Server conventions

- Standard library only unless there's a strong reason; ask before adding a dependency.
- All config via `RIFFLE_*` env vars (`server/config.go`); document new ones in `server/README.md`.
- Never transcode on the default path. Streaming must stay `http.ServeContent` on the
  original file so Range requests and sendfile keep working (the logging middleware's
  `statusRecorder` implements `ReadFrom` for this reason).
- Trickplay layout (10x10 sheets, `NNN.jpg`, manifest fields) is a contract with the
  iOS client; change it in both places.
- Library state is `library.json` and `progress.json` in `RIFFLE_DATA`, written atomically.

## Checks

```sh
cd server && make test   # gofmt, vet, race build, go test -race
```

For a manual run: `make media` then `make dev` (CPU decoding, token `dev`), and
`make diag` to hit `/api/diagnostics`. Releases go through `make release` (see
`server/README.md`); the version comes from `server/VERSION` via `-ldflags`.

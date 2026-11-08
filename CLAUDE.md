# Riffle

Personal iOS video player that streams from the owner's Unraid NAS. **Scrubbing
performance is the top priority.** Read `docs/architecture.md` for decisions and status.

## Layout

- `server/`: Riffle Server (Go 1.24, standard library only, shells out to ffmpeg/ffprobe).
  Runs in Docker on Unraid. API documented in `server/README.md`.
- `ios/`: Riffle iOS app (not started yet). Plan: SwiftUI + MPVKit (libmpv).
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
cd server && gofmt -l . && go vet ./... && go build -race -o /tmp/riffle .
```

For a manual run, generate test media with ffmpeg (`testsrc`, a few codecs: h264 mkv,
hevc mp4, wmv2 wmv) and run with `RIFFLE_HWACCEL=none`.

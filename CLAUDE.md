# Riffle

Personal iOS video player that streams from the owner's Unraid NAS. **Scrubbing
performance is the top priority.** Read `docs/architecture.md` for decisions and status.

## Layout

- `server/`: Riffle Server (Go 1.24, standard library only, shells out to ffmpeg/ffprobe).
  Runs in Docker on Unraid. API documented in `server/README.md`.
- `ios/`: Riffle iOS app (not started yet). Plan: SwiftUI + MPVKit (libmpv).
- `design/`: design system. `tokens.json` is the source; `RiffleTheme.swift` mirrors it
  for SwiftUI (change both together); `README.md` has the usage rules the app follows;
  `components/` has a guideline per app control; `player.md` specifies the player
  (orientation follows the video, gestures, scrubbing). Build iOS UI from these.
- `docs/`: architecture and decisions.

## Git workflow

Several agent sessions work in this repo at once, and the main checkout
(`~/Projects/riffle`) is the owner's. One session once committed another's untracked work
with `git add -A`, then a branch switch removed those files from disk. To avoid that:

- **Work in your own worktree.** Start each task with
  `git worktree add ../riffle-<task> -b <branch> origin/main` and make every edit, build
  and commit there. Don't edit, commit or switch branches in the main checkout. Remove the
  worktree once the PR is merged (`git worktree remove ../riffle-<task>`). For subagents,
  use the Agent tool's `isolation: "worktree"`.
- **Stage files by name** (`git add server/foo.go`). Never `git add -A`, `git add .` or
  `git commit -a`, and read `git status` before each commit.
- **Leave others' files alone.** Files you didn't create or change in this task aren't
  yours: don't commit, stash, move or delete them, even if they're untracked.
- **One PR per task**; the owner merges. Releases (`cd server && make release`) run from
  the main checkout on an up-to-date `main`; they only commit `server/VERSION`.
- A new worktree has no `server/testdata/` or `server/data/` (both gitignored): run
  `make media` there. If another session's server is on port 8770, use
  `RIFFLE_ADDR=:8771 make dev`.

## Server conventions

- Standard library only unless there's a strong reason; ask before adding a dependency.
- The admin dashboard (`server/admin/`, served at `/admin/` and `/admin/library`) is
  `html/template` fragments (`admin/fragments*.html`) driven by htmx 2.0.4, with shared
  `admin.css`/`admin.js`, all vendored and embedded with `go:embed`. No build step; keep it
  that way. Restart the server to see edits: the files are compiled into the binary.
  It follows `design/`: the lockup, illustrations and favicons in `server/admin/` are
  copies from `design/assets/` (go:embed can't reach outside `server/`), so update them
  together; `admin/icons.svg` holds outlined stand-ins for the SF Symbols in
  `design/README.md`, used through the `icon` template helper.
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

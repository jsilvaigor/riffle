# Riffle Server

Companion server for the Riffle iOS player. It runs on the NAS and:

- indexes your video folders and reads metadata with `ffprobe`
- streams the **original files** over HTTP with byte-range support (no transcoding;
  the iOS app decodes everything), so a seek is a single small request
- pre-renders **trickplay sheets** (grids of thumbnails) so scrubbing previews are
  instant and never wait on the network
- stores resume positions

Written in Go using only the standard library, plus `ffmpeg`/`ffprobe` at runtime.

## Configuration

All settings are environment variables.

| Variable | Default | Description |
|---|---|---|
| `RIFFLE_MEDIA` | `/media` | Folders to index: `Movies=/media/movies,TV=/media/tv`, or plain paths |
| `RIFFLE_DATA` | `/data` | Index, progress and thumbnail cache. **Put this on an SSD.** |
| `RIFFLE_TOKEN` | *(empty)* | Shared secret. Empty means no auth (a warning is logged) |
| `RIFFLE_ADDR` | `:8770` | Listen address |
| `RIFFLE_HWACCEL` | `cuda` | Thumbnail decoding: `cuda` (NVIDIA), `vaapi` (Intel iGPU) or `none` |
| `RIFFLE_TRICKPLAY_INTERVAL` | `5` | Seconds between thumbnails (short clips get finer spacing) |
| `RIFFLE_TRICKPLAY_WIDTH` | `320` | Thumbnail width in px |
| `RIFFLE_TRICKPLAY_WORKERS` | `1` | Concurrent thumbnail jobs. Keep at 1 for HDD arrays |
| `RIFFLE_TRICKPLAY_ACCURATE` | `false` | `true` decodes every frame for exact thumbnails (much slower) |
| `RIFFLE_SCAN_MINUTES` | `30` | Periodic rescan; `0` disables |

By default thumbnails come from **keyframes only** (`-skip_frame nokey`). That's 10-50x
faster than a full decode; each thumbnail snaps to the nearest keyframe, usually under
2-5 s away. Clips under 2 minutes are decoded in full (cheap, and they often have a single
keyframe), and any keyframe-only pass that fails is retried with a full decode. Items that
failed are retried once each time the server starts. If hardware decoding fails because the device is missing, the server
switches to the CPU for the rest of the session.

## Running on Unraid

1. Pull the image published by `make release` (Unraid terminal, once `docker login
   registry.isilva.dev` is done):
   ```sh
   docker pull registry.isilva.dev/riffle-server:latest
   ```
   To build on the server instead, clone the repo and run `docker build -t riffle-server .`
   in `server/`.
2. Make sure the **Nvidia Driver** plugin is installed (needed for `--runtime=nvidia`).
3. Run it:
   ```sh
   docker run -d --name riffle-server --restart unless-stopped \
     --runtime=nvidia -e NVIDIA_VISIBLE_DEVICES=all \
     -p 8770:8770 \
     -e RIFFLE_MEDIA="Movies=/media/movies,TV=/media/tv" \
     -e RIFFLE_TOKEN="change-me" \
     -v /mnt/user/Media/Movies:/media/movies:ro \
     -v /mnt/user/Media/TV:/media/tv:ro \
     -v /mnt/cache/appdata/riffle:/data \
     registry.isilva.dev/riffle-server:latest
   ```
   Adjust the media paths to your shares. Mount media **read-only**. Point `/data` at
   the SSD pool (`/mnt/cache/...` or your "speed" share) so thumbnails and the index
   stay fast even while array disks are spun down.
4. Check the logs with `docker logs -f riffle-server`. If you see
   `cuda device unavailable, using CPU`, the NVIDIA runtime isn't reaching the container.
5. Check hardware decoding from any machine: `make diag URL=http://tower:8770` (the token
   comes from `TOKEN=`, `$RIFFLE_TOKEN` or `~/.config/riffle/token`)
   (or `curl -H "Authorization: Bearer change-me" http://tower:8770/api/diagnostics`).
   See [Diagnostics](#diagnostics) for how to read it.

To use the Intel iGPU instead: set `RIFFLE_HWACCEL=vaapi`, add `--device /dev/dri`, and
drop the `--runtime=nvidia` flags.

The first scan of a large library runs ffprobe on every file, which takes a few minutes on
spun-down disks. Items are queued for thumbnails as soon as each is probed, the index is
saved every 15 s, and the log shows `scan: probed N/M`.

### Spun-down disks

Unraid spins array disks down, and the first read can take 5-10 s. The app should call
`POST /api/items/{id}/warm` when the user opens a video's detail screen; this reads a
little of the file so the disk is spinning by the time they press play. Thumbnails come
from the SSD, so previews work right away regardless.

## API

All endpoints except `/api/health` require the token, sent as `Authorization: Bearer <token>`,
`X-Riffle-Token: <token>`, or `?token=<token>` (for players that can't set headers).

| Method & path | Description |
|---|---|
| `GET /api/health` | Server name, version, commit, whether auth is required |
| `GET /admin/` | Admin dashboard (see below); its fragments and actions under `/admin/f/` and `/admin/a/` need the token |
| `GET /api/diagnostics` | Build, ffmpeg/GPU environment and live decode tests (takes 1-3 s) |
| `GET /api/library` | `{roots: [...], items: [...]}` with metadata and trickplay state. Gzipped when accepted; weak `ETag`, so a refresh with `If-None-Match` gets a 304 |
| `POST /api/library/rescan` | Trigger a rescan (202) |
| `GET /api/items/{id}` | One item |
| `GET`/`HEAD /api/items/{id}/stream` | Original file, Range-enabled |
| `POST /api/items/{id}/warm` | Spin up the disk ahead of playback (202) |
| `GET /api/items/{id}/poster.jpg` | Still for the library screen: the trickplay tile 10% into the video (tile size, 320 px wide by default). 202 + `Retry-After` until trickplay is ready (doesn't jump the queue); 404 if unavailable |
| `GET /api/items/{id}/trickplay/manifest.json` | 200 manifest; 202 + `Retry-After` while generating (jumps the queue); 404 if unavailable |
| `GET /api/items/{id}/trickplay/{n}.jpg` | Sheet `n` (0-based) |
| `GET /api/progress` | All resume positions (gzip and `ETag` like the library) |
| `PUT /api/items/{id}/progress` | `{"position": 123.4, "duration": 5400}` |

### Video metadata

Each item's `probe.video` carries the stored `width` and `height`, plus:

- `rotation`: degrees the stored frame turns clockwise to display (0, 90, 180, 270). Phones
  store most portrait clips as landscape pixels with a rotation, so `width` > `height`
  doesn't mean the video is landscape.
- `displayWidth`, `displayHeight`: the size it's shown at, after rotation and non-square
  pixels. The iOS player picks its orientation from these (see `design/player.md`).

ffmpeg and mpv apply the rotation themselves, so thumbnails and playback are already
upright. Trickplay tiles take the display shape too: a portrait phone clip gets 9:16 tiles
and an anamorphic DVD rip 16:9 ones. Items indexed by an older server are re-probed once on
the next scan (`probe.v` tracks this, and the log and `/api/diagnostics` count them as
`reprobe`); their thumbnails are kept unless the tiles don't match the display shape, as
with anamorphic videos thumbnailed before 0.2.2.

### Trickplay manifest

```json
{ "interval": 5, "tileWidth": 320, "tileHeight": 180, "columns": 10, "rows": 10,
  "count": 1440, "sheets": 15, "format": "jpg" }
```

Thumbnail `k` shows time `k * interval`. It's in sheet `k / (columns*rows)`, at cell
`k % (columns*rows)` counting left to right, top to bottom. A 2-hour movie is about
15 sheets, roughly 5-7 MB in total, so the app can prefetch all of them when a video opens.

### Admin dashboard

Open `http://<server>:8770/admin/` in a browser. It asks for the token once and keeps it in
that browser's localStorage ("Change token" in the header forgets it). It uses the Riffle
design system: the lockup, the app icon as favicon (also served at `/favicon.ico`),
stand-ins for the app's SF Symbols, and the empty-state illustrations. The page is built with
[htmx](https://htmx.org) 2.0.4, vendored in `admin/` and embedded in the binary, so it works
without internet access.

It refreshes on its own (overview and jobs every 5 s, problems every 30 s, storage and
breakdown every minute) and shows:

- **Server:** version, uptime, the decoder in use (and whether the GPU fell back to CPU)
- **Library:** video count, size, running time, resume positions, media roots
- **Scan:** idle or probing N of M; **Rescan** and **Re-probe all** (re-reads every file's
  metadata, keeping thumbnails)
- **Thumbnails:** progress, queue, failures; **Pause**/**Resume** (not kept across restarts),
  **Retry failed**, **Clean cache** (removes thumbnails of deleted videos, unfinished jobs,
  cached posters and the diagnostics clip) and **Rebuild all** (deletes every thumbnail and
  queues the library again)
- **Recent thumbnail jobs**, with the decoder and time each took
- **Problems:** files whose metadata couldn't be read (**Re-probe**) and thumbnail failures
  with ffmpeg's error (**Retry**, ahead of the queue)
- **Storage** used in `RIFFLE_DATA`, and a **library breakdown** by codec, container,
  resolution and orientation
- **Hardware tests:** the `/api/diagnostics` decode tests, run on demand

The **Library** page (`/admin/library`) browses the library without playing anything: a
grid of posters (60 at a time, "Show more" for the rest) with search, a folder filter
(subfolders included), filters for thumbnail state, unreadable metadata, portrait videos
and started videos, and sorting by name, recently changed, size or duration. Clicking a
video opens its details: a hover-scrub preview built from the trickplay sheets (the same
frames the app shows while scrubbing), the sheets themselves, metadata (codec, stored and
displayed size, rotation, audio and subtitle tracks), the resume position, and **Rebuild
thumbnails** / **Re-probe**. Details are linkable as `/admin/library#item=<id>`. Images
load through the API with the token as `?token=`, since `<img>` can't send headers.

### Diagnostics

`GET /api/diagnostics` answers "is the GPU actually doing the work?" without shell access
to the container. It generates a 20 s 720p h264 clip in `RIFFLE_DATA/diagnostics/` (once)
and decodes it several ways:

| Test | Passes when |
|---|---|
| `none` | ffmpeg can decode on the CPU |
| `cuda` | NVDEC decodes with frames kept on the GPU, so a silent CPU fallback counts as a failure |
| `vaapi` | Same on the Intel iGPU; skipped without `--device /dev/dri` |
| `trickplay:<hwaccel>` | The real trickplay command works with the configured `RIFFLE_HWACCEL` |

It also reports scan progress under `scan` (files found, probed so far), `nvidia-smi` output (only present with `--runtime=nvidia`), the `/dev/dri`
nodes, `ffmpeg -hwaccels`, and under `trickplay` the decoder in use (`effectiveHWAccel`,
`hwDisabled` once a device failure switched it to CPU) and the last 20 jobs with the
decoder and time each took. On the RTX 3050 expect `cuda` and `trickplay:cuda` to pass
and recent jobs to show `"hwaccel": "cuda", "fallback": false`.

## Development

Everything runs through the `Makefile` in `server/` (`make help` lists the targets):

```sh
make test          # gofmt, vet, race build, unit tests
make media         # test videos in testdata/media: h264 mkv, hevc mp4, wmv2 wmv, short portrait,
                   # phone portrait (rotation flag), anamorphic mkv; skips files that exist
make dev           # run natively on CPU against testdata/media, token "dev"
make build run     # same in Docker (docker-compose.yml), then `make logs` / `make stop`
make diag          # call /api/diagnostics on localhost (URL= and TOKEN= to point elsewhere)
```

### Releasing

`make release` (or `make release BUMP=minor|major`) needs a clean `main` that matches
`origin/main`. It runs the tests, bumps `VERSION`, commits and tags `server-vX.Y.Z`,
builds a `linux/amd64` image with the version and commit baked in (shown by `/api/health`),
and only after a successful build pushes the commit, the tag, and
`registry.isilva.dev/riffle-server:X.Y.Z` and `:latest`. Override the registry with
`IMAGE_NAME=...`.

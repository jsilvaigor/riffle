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
2-5 s away. If hardware decoding fails because the device is missing, the server
switches to the CPU for the rest of the session.

## Running on Unraid

1. Build the image on the server (Unraid terminal):
   ```sh
   git clone https://github.com/jsilvaigor/riffle.git /mnt/user/appdata/riffle-src
   cd /mnt/user/appdata/riffle-src/server
   docker build -t riffle-server .
   ```
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
     riffle-server
   ```
   Adjust the media paths to your shares. Mount media **read-only**. Point `/data` at
   the SSD pool (`/mnt/cache/...` or your "speed" share) so thumbnails and the index
   stay fast even while array disks are spun down.
4. Check the logs with `docker logs -f riffle-server`. If you see
   `cuda device unavailable, using CPU`, the NVIDIA runtime isn't reaching the container.

To use the Intel iGPU instead: set `RIFFLE_HWACCEL=vaapi`, add `--device /dev/dri`, and
drop the `--runtime=nvidia` flags.

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
| `GET /api/health` | Server name, version, whether auth is required |
| `GET /api/library` | `{roots: [...], items: [...]}` with metadata and trickplay state |
| `POST /api/library/rescan` | Trigger a rescan (202) |
| `GET /api/items/{id}` | One item |
| `GET`/`HEAD /api/items/{id}/stream` | Original file, Range-enabled |
| `POST /api/items/{id}/warm` | Spin up the disk ahead of playback (202) |
| `GET /api/items/{id}/trickplay/manifest.json` | 200 manifest; 202 + `Retry-After` while generating (jumps the queue); 404 if unavailable |
| `GET /api/items/{id}/trickplay/{n}.jpg` | Sheet `n` (0-based) |
| `GET /api/progress` | All resume positions |
| `PUT /api/items/{id}/progress` | `{"position": 123.4, "duration": 5400}` |

### Trickplay manifest

```json
{ "interval": 5, "tileWidth": 320, "tileHeight": 180, "columns": 10, "rows": 10,
  "count": 1440, "sheets": 15, "format": "jpg" }
```

Thumbnail `k` shows time `k * interval`. It's in sheet `k / (columns*rows)`, at cell
`k % (columns*rows)` counting left to right, top to bottom. A 2-hour movie is about
15 sheets, roughly 5-7 MB in total, so the app can prefetch all of them when a video opens.

## Development

```sh
cd server
go build -o riffle-server .
RIFFLE_MEDIA=/path/to/videos RIFFLE_DATA=/tmp/riffle-data RIFFLE_HWACCEL=none ./riffle-server
```

# Riffle — Architecture

Personal iOS video player that streams from a home NAS, with scrubbing performance as
the top priority. Two parts:

- **Riffle Server** (`server/`): companion that runs in Docker on the NAS
- **Riffle for iOS** (`ios/`, see `ios/README.md`): the player

## Requirements

1. Play common formats: mp4, mov, wmv, mkv, avi, ts, etc.
2. Stream from the NAS over the fastest practical protocol.
3. **Scrubbing performance first:** responsive seeking and timeline previews over the network.

## Target hardware

- Unraid; Intel i5-9600K (UHD 630 iGPU), 16 GB RAM, NVIDIA RTX 3050
- Media on the HDD array; SSD "speed" share / cache pool for appdata

## Decisions

| Decision | Choice | Why |
|---|---|---|
| Transport | HTTP with byte ranges from a companion server | A seek is one range request; sendfile keeps server cost near zero. Simpler and faster than SMB/NFS from iOS |
| Transcoding | None by default (direct play) | The client decodes everything; no quality loss or server CPU cost |
| Client decoder | MPVKit (libmpv) | AVFoundation can't play mkv/wmv; mpv has hardware decoding and fast seeking |
| Scrub previews | Pre-rendered trickplay sheets, keyframes only, NVDEC with CPU fallback | Previews need zero network while dragging; a 12-min 720p file takes ~1.6 s on CPU |
| Server language | Go, standard library only | Single static binary, good HTTP stack, easy Docker image |
| Server state | JSON files in `/data` (SSD) | Fine for a personal library; no database dependency |
| Discovery | Manual address for now; Bonjour later | Bonjour needs a third-party Go library |
| Remote access | Tailscale/WireGuard, no open ports | Safer than exposing the server |
| Player orientation | Follows the video: portrait plays portrait; only wide videos request landscape, once | Portrait phone clips must not be forced sideways. The server reports display size after rotation, so the app decides before the first frame (`design/player.md`) |

## Scrubbing model (client)

1. When a video opens, prefetch the manifest and all sheets (about 5-7 MB for 2 hours).
2. While dragging, show the sprite cell for the finger position; no network involved.
   Optionally issue throttled keyframe seeks to mpv for live video feedback.
3. On release: keyframe seek (`hr-seek=no`) for instant resume, then a precise seek.
4. Call `warm` from the details screen to hide HDD spin-up.

## Status

- [x] Server: scanning, ffprobe metadata, range streaming, trickplay queue (priority for
  items being watched), progress, token auth, Dockerfile, Unraid instructions
- [x] Server: Makefile, docker-compose for local runs, versioned releases to
  `registry.isilva.dev/riffle-server`, basic Go tests, `/api/diagnostics`
- [x] Server: `-hwaccel cuda` verified on the RTX 3050 inside the container (driver
  615.78.08, ~0.5 s per 720p trickplay job); VAAPI on the UHD 630 works too
- [x] Server: first scan queues trickplay per probed file and saves the index as it goes
- [x] Server: poster endpoint (cropped trickplay tile), gzip + ETag on library/progress,
  trickplay for short single-keyframe clips
- [x] Server: admin dashboard at `/admin/` (htmx, embedded): stats, scan and thumbnail
  actions, problems list, hardware tests
- [x] Server: watch states (`unwatched`/`partial`/`watched`, sticky, mark and unmark,
  `finishedAt`) decided server-side for the app's counters; dashboard filters and counts
- [ ] Server: Bonjour advertisement, Unraid template XML
- [x] Server: rotation and display size in item metadata, for player orientation
- [x] iOS app: Xcode project, connect screen, library browser, detail, search, settings
- [x] iOS app: MPVKit player, custom scrub bar with trickplay, gestures, orientation,
  track sheet, progress reporting
- [x] Server + iOS: baked-in black bands detected during trickplay (`cropdetect`) and
  cropped in the player, previews and posters
- [ ] iOS app: Picture in Picture, AirPlay remote mode; verify on a device

# VideoCard

A 16:9 thumbnail with title and metadata, for grids and shelves such as "Continue watching".

## Provide
- `title`: the cleaned-up name ("Salt Road · S01E03 · Low Tide"); fall back to the file name without extension.
- `meta`: one `caption` line: folder and size, or show and episode.
- `thumbnail`: a frame from the trickplay sheets (about 10% in avoids black openings). Omitted, the card shows the film glyph on `surface-raised`.
- `duration` in seconds (chip, bottom right, `timecode` on `scrim`).
- `progress` 0–1 from the server's resume position: a `citron` bar along the bottom edge, shown while the video is in progress (state `partial`, or a watched video being watched again).
- `badges`: at most two, most important first (4K before HDR before codec).
- `loading`: a placeholder with no spinner.

## Rules
- Thumbnails are always 16:9 with `radius-md`; crop, never letterbox.
- Titles wrap to two lines, then truncate. Meta is one line.
- Grid gap is `space-2` on iPhone and `space-4` on iPad.

**In the app:** a `LazyVGrid` cell; thumbnails cached locally with the trickplay sheets.

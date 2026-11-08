# LibraryRow

A list row for browsing the library: folders, and video files with their format details.

## Provide
- `kind`: `folder` or `video`.
- `title`: folder name, or the video's title (fall back to the file name, extension included, so mkv and wmv files are recognisable).
- Folder: `count` of items inside.
- Video: `meta` ("1:52:10 · 4.2 GB"), `badges` for resolution and codec from ffprobe, `thumbnail`, `progress` 0–1.
- `error`: replaces the metadata with an `alert` line and icon when the server couldn't probe the file ("Couldn't read this file").
- `onPress`.

## Rules
- Folders first, then videos, each alphabetical.
- Folder rows use `body` with a folder glyph and a trailing count and chevron; video rows use `headline` with a 96 pt-wide 16:9 thumbnail at `radius-sm`.
- Hairlines in `line` start after the leading image, iOS style; the last row has none.
- A row with an error stays tappable: it opens details with the reason.

**In the app:** a `List` row; SF Symbols `folder`, `chevron.right`, `exclamationmark.triangle`.

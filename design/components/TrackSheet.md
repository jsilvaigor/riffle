# TrackSheet

A bottom sheet for choosing the audio track and subtitles of the current video.

## Provide
- `audio`: the audio streams from the item's ffprobe tracks, each `{id, label, detail}`. Label is the language in its own name ("Português (Brasil)"); detail is codec and channels ("AC3 5.1").
- `subtitles`: the same for subtitle streams; detail is the format plus "forced" when flagged. "Off" is added first.
- `selectedAudio`, `selectedSubtitle`, `onSelectAudio`, `onSelectSubtitle`, `onClose`.

## Rules
- Selection applies immediately, without closing the sheet; "Done" closes it.
- The selected row is semibold with an `ink` check. The only `citron` is "Done".
- Rows are at least 44 pt, separated by `line` hairlines. Section labels are `caption` in `ink-muted`.
- Name a track by its title tag when the file has one ("Commentary"), otherwise by language.

**In the app:** `.sheet` with `.presentationDetents([.medium, .large])`; `checkmark` for the check.

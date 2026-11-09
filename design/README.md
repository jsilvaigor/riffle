# Riffle design system

Usage rules for the Riffle iOS app. The tokens live in [`tokens.json`](tokens.json) and are mirrored for SwiftUI in [`RiffleTheme.swift`](RiffleTheme.swift); change both together. Component guidelines are in [`components/`](components/), the player's behaviour (orientation, gestures, scrubbing) in [`player.md`](player.md); logos, app icons, illustrations and patterns in [`assets/`](assets/). The editable version, with the visual cover and token views, is the Riffle design system artifact on claude.ai.

Riffle is a personal video player for iPhone and iPad that streams your own library from Riffle Server on your NAS. The video is the interface; everything Riffle draws is there to get out of its way, then come back the instant you reach for it. The system is dark-first, quiet, and spends its one accent, `citron`, on the thing you touch most: the scrubber.

## Principles

- **Picture first.** Chrome hides after three seconds of playback and returns on tap. Never cover more of the frame than the controls need.
- **One accent.** `citron` marks progress and the single most important action on a screen. Everything else is `ink`, `ink-muted` and the surfaces.
- **Fast to flick through.** Scrubbing, skipping and resuming are one gesture each. Times are always visible while scrubbing, set in `timecode`.
- **Native, not novel.** Follow iOS conventions: system font, SF Symbols, standard navigation, 44 × 44 pt minimum hit targets.

## Content fundamentals

- **Voice:** brief, calm, plain. Talk to the viewer as "you"; Riffle never says "I" or "we" in the interface.
- **Casing:** sentence case for titles, buttons and menu items ("Continue watching", "Rescan library"). Uppercase only in `label` badges (4K, HDR, HEVC, 5.1, CC).
- **Verbs on buttons:** "Play", "Resume", "Rescan library", "Try again"; not "OK" or "Submit".
- **Names:** call the companion "Riffle Server" and the places it indexes "folders". Show file names with their extension when there's no better title, so formats stay recognisable.
- **Times:** `m:ss` under an hour, `h:mm:ss` over it ("4:07", "1:04:37"). Elapsed left of the scrubber, remaining right with a minus sign ("−12:40").
- **Errors** say what happened and what to do: "Couldn't load this video. Check that Riffle Server is running and try again." Waits that have a cause say so: "Waking up the disk…".
- **No emoji** in the interface, and no exclamation marks.

## Color

There is one theme, Dark. Video looks best against black, and the dark surfaces keep attention on the frame.

- Fill the space around the video with `stage`. Use `surface` for every other screen's background, and `surface-raised` for cards, sheets, menus and notices.
- Set primary text and icons in `ink`, and secondary text (duration, size, codec, folder) in `ink-muted`. `ink` reads on `surface`, `surface-raised`, `stage` and `scrim`; `ink-muted` on the first three only, so in the player secondary text is `ink` too.
- Use `citron` for: the played part of the scrubber and its thumb; the primary Play or Resume button; the selected tab; links; the focus ring. Label anything on a `citron` fill in `on-citron`. Outside the scrubber, allow one `citron` element per view.
- Lay `scrim` over the video whenever player controls show. Controls never sit on the bare frame.
- Use `alert` only for errors (an unreadable file, a video that won't load, an offline server), and always pair it with a word or an icon so it never relies on colour alone.
- `line` is for decorative hairlines and the unplayed scrubber track. It is too faint for a control border: outline controls in `ink-muted`.

## Type

Riffle uses the iOS system font (SF Pro) through the `sans` family, so it follows the viewer's device and Dynamic Type. Treat the sizes below as the Large (default) Dynamic Type step and scale with it.

- `display`: the large navigation title, once per screen.
- `title`: section headers ("Continue watching", "Movies") and the video title on its detail page.
- `headline`: video titles on cards and rows, sheet titles, button labels.
- `body`: descriptions, settings and dialogs.
- `caption`: metadata under a title, always in `ink-muted`.
- `label`: uppercase badges only.
- `tab`: tab bar labels only.
- `timecode` (the `mono` family): every time readout, so digits don't shift while the video plays. On iOS, SF Pro with tabular figures is an acceptable substitute.

## Spacing and layout

- Screen side margins are `space-4`. Sections on a scrolling screen sit `space-6` apart; rows in a list `space-4` apart; a title and its caption `space-2` apart.
- Thumbnails are 16:9 and keep that ratio at every size. In grids, gap them by `space-2` (iPhone) or `space-4` (iPad).
- In the player, transport controls (skip back, Play/Pause, skip forward) are centred on the frame with `space-6` between them. The scrubber runs along the bottom edge inside the safe area, with `timecode` readouts at either end.
- Every tappable thing has a 44 × 44 pt hit area, even when it draws smaller. The scrubber's visible track is 4 pt tall; its hit area is 44 pt.

## Shape

- `radius-md` on buttons, thumbnails, cards, sheets, notices and the scrub preview frame. The video itself is square-cornered in full screen.
- `radius-sm` on badges and duration chips.
- `radius-pill` on the Play button and other round transport controls, the scrubber track and thumb, chips and segmented controls.
- No shadows. Separate layers by surface (`surface` → `surface-raised`) and, where needed, a `line` hairline.

## The scrubber

The scrubber is Riffle's signature element. Draw it as a `radius-pill` track: unplayed in `line`, buffered in `ink-muted`, played in `citron`, with a `citron` thumb. While dragging, the thumb grows and the target frame appears above it in a `radius-md` frame with the time under it in `timecode`. That frame comes from the trickplay sheets Riffle Server pre-renders, prefetched when the video opens, so dragging never waits on the network. See the Scrubber component.

## States

- **Pressed:** dim the control to 60% opacity; no colour change.
- **Disabled:** 35% opacity; never hide a control the viewer expects to find.
- **Focus** (keyboard, Switch Control, pointer on iPad): a solid 2 pt `citron` ring, offset 2 pt, at 15:1 on `surface` and 4.6:1 on `scrim`.
- **Loading:** a `surface-raised` placeholder at the thumbnail's 16:9 size; no spinners on thumbnails. In the player, a single ring spinner in `ink` at the centre of the frame.

## Motion

Controls fade in and out over 200 ms. Sheets use the standard iOS spring. Respect Reduce Motion: replace slides and scales with fades.

## Iconography

Use SF Symbols throughout, in the weight that matches the adjacent text (Regular beside `body`, Semibold beside `headline`). Use filled variants for transport controls and outlined variants elsewhere. Icons take `ink` or `ink-muted`; the Play glyph on a `citron` button takes `on-citron`; the selected tab takes `citron`.

SF Symbols can't ship outside Apple platforms, so the component previews draw simple stand-in glyphs. Each maps to one symbol:

| Preview glyph | SF Symbol | Where |
| --- | --- | --- |
| play, pause | `play.fill`, `pause.fill` | Transport, Resume button |
| back10, forward10 | `gobackward.10`, `goforward.10` | Transport |
| close | `xmark` | Player |
| replay | `arrow.counterclockwise` | Transport, after the video ends |
| rotate | `rotate.right` | Player |
| rotationLock | `lock.rotation` | Player |
| tracks | `text.bubble` | Player: audio and subtitles |
| pip | `pip.enter` | Player |
| folder, film | `folder`, `film` | Library rows, empty thumbnails |
| chevronRight | `chevron.right` | Folder rows |
| library, search, settings | `rectangle.stack`, `magnifyingglass`, `slider.horizontal.3` | Tab bar |
| server | `server.rack` | Server status |
| check | `checkmark` | Track sheet |
| alert, info | `exclamationmark.triangle`, `info.circle` | Errors, notices |

## Components

Each control the app needs has a guideline in [`components/`](components/): Button, TransportButton, Scrubber, PlayerControls, TrackSheet, VideoCard, LibraryRow, Badge, TabBar, ServerStatus and Notice. Build them in SwiftUI from those rules and the tokens in `RiffleTheme.swift`. Live previews of each are in the design system artifact on claude.ai.

## Logo

The mark is a play triangle sliced into five bands, with alternate bands slid back like riffled cards. The wordmark is lowercase and monoline; its i-dot is the `citron` scrubber thumb. Files and inks are in [`assets/Logos`](assets/Logos/README.md).

- Use `riffle-mark` (`citron`) on dark grounds, `riffle-mark-dark` on `citron` or light grounds, and `riffle-mark-ink` when something else nearby is already `citron`.
- Use the lockup where the name needs to read (About, the server's web page, docs) and the mark alone where space is tight or the name is already present (the app icon, the launch screen).
- Clear space is half the mark's height. The mark is never smaller than 16 pt.
- Never retype the wordmark, rotate the mark, or add outlines or shadows.

## App icon and launch

The app icon is the `surface` mark on a `citron` field. iOS's dark and tinted appearances have their own files. The launch screen is `surface` with the `citron` mark centred at about 115 pt and nothing else, so the first frame of the app matches it.

## Illustrations and patterns

Empty states (no videos, no server, offline, previews in progress) each get one flat illustration from [`assets/Illustrations`](assets/Illustrations/README.md), with a `title` line and a `body` message under it. The riffle tile is a texture for onboarding and marketing only; it never sits behind reading text or in the player.

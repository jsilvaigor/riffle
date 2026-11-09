# Player

How the full-screen player behaves: orientation, chrome, gestures, scrubbing and its states. The look of each control is in [Scrubber](components/Scrubber.md), [PlayerControls](components/PlayerControls.md) and [TransportButton](components/TransportButton.md); this section is how they act together.

## Orientation: the player follows the video

Riffle never forces a video into an orientation it wasn't shot in. **A portrait video plays in portrait.**

| Video (display aspect) | iPhone held upright | iPhone held sideways |
| --- | --- | --- |
| Portrait, under 0.9 (9:16 phone clips) | Fills the width; chrome sits in the bars above and below the picture. | Plays upright in the middle with `stage` bars at the sides. No rotation is requested. |
| Square, 0.9–1.1 | Fits the width. | Fits the height. |
| Landscape, over 1.1 | Opens by turning to landscape (one request when the player opens). Turning the phone back to upright shows it letterboxed, and it stays that way. | Fills the screen. |

- **Decide before the first frame.** Use the server's `displayWidth` and `displayHeight`: they already account for rotation metadata and non-square pixels. Phones store most clips as 1920 × 1080 pixels plus a 90° rotation flag, so the stored `width` and `height` would wrongly say landscape. If the item has no display size (an index from an older server), wait for mpv's `video-params/dw` and `dh` before choosing, with the screen held on `stage`.
- **Only landscape videos ever request a rotation**, once, on open, and only on iPhone. Settings has "Turn to landscape for wide videos", on by default; off means the player never requests any rotation.
- **After opening, the player follows the phone.** It supports every orientation and never snaps back.
- **Rotate** in the top bar (iPhone only) turns the player between portrait and landscape, for this video only. It works with the system's portrait lock on, which is how most people hold their phone: the player stays where Rotate put it until it's tapped again or the player closes, and the library is still portrait afterwards. With Rotation lock on, Rotate moves the lock to the new orientation.
- **Rotation lock** in the top bar pins the current orientation until the player closes. It works alongside the system's portrait lock; when the system lock is on, the landscape request still applies because the person opened a wide video, but nothing rotates after that.
- **iPad** never requests a rotation; the player fits whatever way the iPad is held.
- **Closing the player** returns to the library in the orientation the library was in (portrait on iPhone).
- **Baked-in black bands** (a 2.40:1 film letterboxed in a 16:9 file, a 4:3 show pillarboxed) are cropped off automatically, so the picture itself is what fits the screen. Riffle Server measures them while making trickplay (the manifest's `crop`); the scrub previews and posters are cropped the same way.
- **Fit and fill:** pinch out to fill the screen (cropping the edges), pinch in to fit the whole frame. Fit is the default for every video and every orientation; the choice lasts until the player closes.
- Thumbnails and trickplay previews are already upright (the server's ffmpeg applies the rotation), so a portrait video's previews are 9:16 and the Scrubber's preview frame is 90 × 160.

## Chrome

- A tap on the video shows or hides the chrome. It fades over 200 ms.
- While playing, chrome hides 3 seconds after the last touch. It stays while paused, buffering, scrubbing, with a sheet open, and whenever VoiceOver or Switch Control is running.
- With chrome hidden, the status bar and home indicator hide too.
- While a video plays (or opens, or buffers), the screen stays on. Paused, ended or closed, Auto-Lock works as usual.
- While scrubbing, the transport controls hide so the preview and the frame underneath stay clear; the top bar stays.

## Gestures

| Gesture | Where | Does |
| --- | --- | --- |
| Tap | Anywhere on the video | Show or hide chrome. |
| Double tap | Left or right third | Back or forward 10 s. Each extra tap within 0.6 s adds 10 s; a label shows the total ("−30 s"). Chrome stays as it was. |
| Drag | The scrubber | Scrub (below). |
| Horizontal pan | Anywhere on the video, chrome shown or not | Scrub relative to the current position: one screen width is 2 minutes, or the whole video if shorter. The scrubber and preview appear while panning. |
| Hold | Right half | Play at 2× while held; a "2×" label shows at the top. |
| Pinch | Anywhere | Fit or fill. |
| Swipe down | Anywhere, when not scrubbing | Close the player (interactive: the video shrinks with the finger). |

Gestures that start on a control (a button, the scrubber) belong to the control.

## Scrubbing

Scrubbing is the reason Riffle exists, and the preview is what makes it fast: it comes from memory, never from the network.

1. **When a video opens**, fetch the trickplay manifest. A `202` means the server is still generating it and has moved it up the queue: retry after `Retry-After` while playback starts anyway. Then fetch every sheet (about 5–7 MB for two hours), four at a time, and decode them into memory. A sheet still in flight shows "Preview not ready" in the frame; the time still shows.
2. **While dragging**, the thumb follows the finger and the preview frame updates on every touch event from the sprite cell for that time. At the same time, send mpv a keyframe seek (`seek <t> absolute+keyframes`) no more often than every 120 ms, and only once the previous seek has finished, so the frame under the scrim follows too. Never queue seeks. If the disk is slow, the preview still leads.
3. **Fine scrubbing:** raising the finger above the track slows the scrub, like Apple's own players. Up to 56 pt above the track is full speed, 56–112 pt half speed, 112–168 pt quarter speed, and above that fine (one eighth). The Scrubber's `scrubLabel` says which ("Half-speed scrubbing"), and each change gives a selection haptic.
4. **On release**, seek to the keyframe at or before the target (`hr-seek=no`) so playback resumes at once, then seek precisely (`absolute+exact`) once that frame shows. Playing stays playing; paused stays paused.
5. **Haptics:** a light tick when the thumb reaches the start or the end, and at chapter marks when the file has them.
6. **Keys** (iPad with a keyboard): ← and → skip 10 s (60 s with Shift), Space plays or pauses, Esc closes.

## States

- **Opening:** `stage`, the title visible, and a spinner only after 400 ms so fast opens don't flash.
- **Buffering:** the spinner replaces Play. After 2 seconds on a first open, add "Waking up the disk…" under it: Unraid spins array disks down, and the detail screen's `warm` call usually hides this. After 15 seconds, swap the spinner for an alert Notice: "Still waiting for Riffle Server" with "Try again".
- **Error:** an alert Notice centred on `stage`, with what happened and what to do: "Couldn't play this video. The file uses a format Riffle can't decode." or "Can't reach Riffle Server. Check that it's running." Actions: "Try again" and "Close".
- **Ended:** the last frame stays with chrome shown; the centre control becomes Replay.
- **Resume:** the player starts where it was asked to (the detail screen offers "Resume from 1:04:37" and "Play from start"), so it never prompts.

## Picture in Picture and AirPlay

- Picture in Picture keeps the video's shape: a portrait clip gets a portrait window.
- Starting AirPlay or an external display shows the video there and turns the phone into a remote: `surface`, the title, the transport controls and the scrubber, in whatever orientation the phone is held.

## Accessibility

- VoiceOver keeps the chrome visible. The scrubber is adjustable (swipe up or down for ±10 s) and reads "1:04:37 of 1:52:10". Double-tap skipping and panning aren't needed under VoiceOver; the buttons cover them.
- Reduce Motion: no skip-tap ripple, no scale on the thumb, and the close gesture fades instead of shrinking.
- Every gesture has an equivalent: skip buttons, the scrubber, the close button, and a VoiceOver custom action "Fit" / "Fill" for pinching.

## What the player needs from Riffle Server

`GET /api/items/{id}`: `probe.duration`; `probe.video.displayWidth`, `displayHeight` and `rotation` for orientation; audio and subtitle tracks for the track sheet; the trickplay state. `GET /api/items/{id}/trickplay/manifest.json` and the sheets for previews. `PUT /api/items/{id}/progress` every 10 seconds while playing and on close.

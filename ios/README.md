# Riffle for iOS

SwiftUI app for iPhone and iPad (iOS 18+) that streams from Riffle Server. UI follows
`design/` (tokens, components, `player.md`); the API is in `server/README.md`.

## Layout

- `Riffle.xcodeproj`: hand-written, uses Xcode's synchronized folders, so files added
  under `Riffle/` or `RiffleTests/` join their target automatically. No project generator.
- `Riffle/Model`: API client and wire types (`RiffleClient`, `Models`), app state
  (`AppModel`), trickplay sprite math, text formatting, folder tree, poster cache.
- `Riffle/Components`: one SwiftUI view per `design/components/` guideline.
- `Riffle/Views`: screens (Connect, Library, Folder, Detail, Search, Settings).
- `Riffle/Player`: the player (`design/player.md`). `MPVPlayer` wraps libmpv from
  MPVKit (LGPL build, pinned to `1.1.0-n9.0.2`), rendering with `gpu-next` over
  MoltenVK into a `CAMetalLayer`. `PlayerModel` holds playback state, scrubbing,
  chrome and progress; `TrickplayStore` loads every trickplay sheet into memory
  when the player opens; `GestureLayer` is the UIKit gesture surface.
- `../design/RiffleTheme.swift` is compiled in directly (not copied), so the tokens
  stay in one place.
- `Info.plist` holds only what build settings can't express: the scene manifest, the
  launch screen and ATS (plain HTTP to the NAS, including Tailscale MagicDNS names).

## Things that look odd but are deliberate

- **UIKit app entry** (`App/RiffleApp.swift`): the app starts from a UIKit app and
  scene delegate that host SwiftUI. SwiftUI's own root controller caches the allowed
  orientations at launch, so the landscape request for wide videos failed
  intermittently. The root controller is always portrait on iPhone; the player is
  presented in `PlayerHostingController`, which answers for its own orientations.
- **mpv waits for the layer**: the file loads only once the Metal layer is in a
  window with a real size. Starting earlier could leave the video black.
- **Rotation nudges mpv**: MPVKit's MoltenVK context reads the drawable size only
  when the video output reconfigures, so after a rotation `MPVPlayer.layerDidResize`
  sets `video-aspect-override` to the video's own aspect, nudged by 0.001%, to force
  one, once MoltenVK has rebuilt the swapchain at the new size.
- **Metal API Validation is off in the shared scheme**: MoltenVK recreates mpv's
  swapchain on every size change, and one frame still in flight on the old swapchain
  gets a drawable of the new size. With validation on, running from Xcode stops with
  "renderTargetHeight must be <= minimum attachment height"; without it (as in an
  installed build) Metal drops the out-of-bounds part of that frame. The view doesn't
  set `drawableSize` during a rotation, so it adds no second mismatch: it waits for
  MoltenVK's new swapchain, then has mpv reconfigure. MPVKit's demo needs validation off
  for HDR too.
- **The simulator decodes in software**: VideoToolbox isn't available there, so mpv
  falls back to the CPU. Devices use hardware decoding.

## Run

```sh
open Riffle.xcodeproj                      # then Run on a simulator or device
xcodebuild -project Riffle.xcodeproj -scheme Riffle \
  -destination 'platform=iOS Simulator,name=iPhone 18 Pro' test
```

Against a local server (`cd server && make dev`, no token), skip the connect screen by
passing the address as a launch argument (it lands in UserDefaults):

```sh
xcrun simctl launch booted dev.isilva.riffle -server.address http://localhost:8770
```

Two debug-only defaults help with simulator checks: `debug.autoplay <item id>` opens
that video at launch, and `debug.keepChrome YES` stops the controls from hiding.

```sh
xcrun simctl spawn booted defaults write dev.isilva.riffle debug.autoplay <item id>
```

The address lives in UserDefaults (`server.address`), the token in the Keychain. The
last library response is cached in Caches so the app opens instantly and offline.

## Status

- [x] Connect, library browser (folders, Continue watching), detail with Resume and
  disk warm-up, search, settings, posters, unit tests for the server contract
- [x] Player: mpv playback, controls, scrubber with in-memory trickplay previews and
  fine scrubbing, throttled keyframe seeks then an exact seek on release, gestures,
  orientation following the video, buffering and error states, track sheet, resume
  position every 10 s and on close
- [x] Watch states from Riffle Server: counts per folder, watched marks, Continue
  watching from the `partial` state, Mark as watched / unwatched
- [x] Running on a device
- Dropped: Picture in Picture, AirPlay remote mode, chapter haptics

# Riffle for iOS

SwiftUI app for iPhone and iPad (iOS 18+) that streams from Riffle Server. UI follows
`design/` (tokens, components, `player.md`); the API is in `server/README.md`.

## Layout

- `Riffle.xcodeproj`: hand-written, uses Xcode's synchronized folders, so files added
  under `Riffle/` or `RiffleTests/` join their target automatically. No project generator.
- `Riffle/Model`: API client and wire types (`RiffleClient`, `Models`), app state
  (`AppModel`), trickplay sprite math, text formatting, folder tree, poster cache.
- `Riffle/Components`: one SwiftUI view per `design/components/` guideline.
- `Riffle/Views`: screens (Connect, Library, Folder, Detail, Search, Settings, Player).
- `../design/RiffleTheme.swift` is compiled in directly (not copied), so the tokens
  stay in one place.
- `Info.plist` holds only what build settings can't express: the launch screen and ATS
  (plain HTTP to the NAS, including Tailscale MagicDNS names).

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

The address lives in UserDefaults (`server.address`), the token in the Keychain. The
last library response is cached in Caches so the app opens instantly and offline.

## Status

- [x] Connect, library browser (folders, Continue watching), detail with Resume and
  disk warm-up, search, settings, posters, unit tests for the server contract
- [ ] Player: MPVKit render view, PlayerControls, Scrubber with trickplay prefetch,
  orientation rules, gestures, progress reporting (`design/player.md`)
- [ ] Track sheet, Picture in Picture, AirPlay remote mode

<h1>
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="design/assets/Logos/riffle-lockup.svg">
    <img alt="Riffle" src="design/assets/Logos/riffle-lockup-dark.svg" height="64">
  </picture>
</h1>

A local-network-first iOS video player, built for fast scrubbing over a home NAS.

- **Riffle Server** ([`server/`](server/)): runs on the NAS (Docker / Unraid). Streams
  original files over HTTP with byte ranges and pre-renders thumbnail sheets for instant
  scrub previews.
- **Riffle for iOS** (`ios/`, coming soon): SwiftUI + libmpv. Plays mkv, mp4, mov, wmv and more.
- **Design system** ([`design/`](design/README.md)): colours, type, spacing and usage rules
  for the app, with a SwiftUI theme.

See [`docs/architecture.md`](docs/architecture.md) for the design.

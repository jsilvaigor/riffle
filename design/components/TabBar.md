# TabBar

The app's bottom navigation between its top-level screens.

## Provide
- `items`: by default Library, Search and Settings. Each `{id, label, icon}`.
- `selected` and `onSelect`.

## Rules
- Three to four tabs. The selected tab's icon and label are `citron`; the rest are `ink-muted`.
- Labels use the `tab` style under a 26 pt icon. No badges or counts on tabs.
- Hidden in the full-screen player.

**In the app:** a standard `TabView` tinted `citron`; SF Symbols `rectangle.stack`, `magnifyingglass`, `slider.horizontal.3`.

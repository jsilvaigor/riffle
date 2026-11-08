# Notice

An inline message on a screen: what happened, and what to do next.

## Provide
- `tone`: `alert` for failures (can't reach the server, file won't play), `neutral` for information (previews still generating).
- `title`: what happened, short ("Couldn't load this video").
- `message`: what to do ("Check that Riffle Server is running and try again.").
- `action`: optional `{label, onPress}`, shown as a plain button ("Try again").

## Rules
- `surface-raised` with `radius-md`; the alert icon is `alert`, the info icon `ink-muted`.
- Sits in the content, not floating over it. One per screen.
- No exclamation marks, no blame, no error codes in the title.

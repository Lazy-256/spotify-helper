## Why

The Noctalia plugin wants a heart button that adds the playing track to the user's library
("Liked Songs") and shows whether it is already there. The helper can only *read* the library
today (`playlists`, `albums`): it requests `user-library-read` and has no command that saves,
removes or checks an item. Without these the plugin cannot offer the button, and the plugin
must not talk to Spotify itself.

## What Changes

- New command `save <uri|link>`: adds a track or episode to the user's library.
- New command `unsave <uri|link>`: removes it again.
- New command `saved <uri|link>`: reports whether the item is in the library, as
  `{"uri":"…","saved":true|false}`.
- `login` requests one more scope, `user-library-modify`. Tokens issued before this change lack
  it, so the user must run `spotify-helper login` once more; `doctor` already reports
  `scopes_missing` for such a token because it compares against the requested scope list.
- `save` and `unsave` check the stored token's scopes **before** calling Spotify and fail with a
  new error code `scopes_missing` (exit 3) when `user-library-modify` is absent, so a caller can
  tell the user to log in again instead of showing a raw 403.
- Calls use Spotify's current library endpoints, which take Spotify URIs: `PUT /me/library`,
  `DELETE /me/library` and `GET /me/library/contains`, each with `?uris=<uri>`.
- The JSON output contract is **extended, not broken**: three new commands, one new error code.
  No existing field changes.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `library`: new requirements for `save`, `unsave` and `saved` (accepted URI kinds, output
  shapes, idempotence, errors).
- `auth`: `login` requests `user-library-modify` as well; a token without it is treated as
  lacking a required scope.
- `output-contract`: new error code `scopes_missing`, exit code 3.

## Non-goals

- Saving or following albums, artists, playlists and shows (the endpoints accept them, but
  nothing needs them yet; they are rejected as `usage`).
- Listing the saved tracks ("Liked Songs" browsing), paging, covers.
- Checking or saving several items in one call (Spotify allows 40; the plugin needs one).
- Any change to `state`: it does not gain a `saved` field, because that would cost an extra
  Spotify request on every poll.
- Re-authorizing automatically; the helper only reports `scopes_missing`.

## Impact

- Code: `internal/auth` (scope list), `internal/api/library.go` (three calls),
  `internal/cli/library.go` (three commands), `internal/output` (error code and exit mapping),
  `internal/spotifyuri` (reuse of `ParseAllowed`).
- Docs: `README.md` and `AGENTS.md` (scope list, command table, error codes) in the same commit.
- Consumer: the plugin change `add-like` in `noctalia-spotify` depends on this one.
- Users: one extra `spotify-helper login` after upgrading.
- Risk: Spotify's Development Mode may restrict the new `/me/library` endpoints; verify by hand
  with the real account before the plugin relies on them.

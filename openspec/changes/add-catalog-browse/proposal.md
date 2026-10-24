## Why

The Noctalia plugin wants artist, album and playlist screens: a header with a play button and a list
of what is inside (an artist's albums, an album's or playlist's tracks), with "add to queue" per
track and "play from here". The helper can search, play a whole album, playlist or artist, and list
the user's own library, but it cannot say what an album, playlist or artist contains, and it cannot
start playback at a given track of a context.

## What Changes

- Three read-only commands that return the contents of one catalog item:
  - `album <uri|link>`: the album and its tracks.
  - `playlist <uri|link>`: the playlist and its tracks. Spotify's Development Mode returns the items
    only of playlists the user owns or collaborates on, so the answer says whether tracks are available
    instead of failing.
  - `artist <uri|link>`: the artist and a page of their albums and singles.
  All three take `--limit` (1–50), `--offset` and `--covers`, and reuse the existing entry shapes
  (`Item`, album, artist and playlist entries) so that every `uri` works with `play-uri` and
  `queue-add`.
- `play-uri <track|episode> --context <album|playlist>` starts the context at that item ("play from
  here"), using the `offset` of Spotify's play request.
- `login` also requests `playlist-read-collaborative`, which collaborative playlists need. Existing
  tokens keep working; the new `playlist` output flags `collaborative` so that a caller can tell the
  user to log in again.

**Does not break the JSON contract**: new commands, new fields on new outputs, and one new optional flag.
No existing output changes. The `login` output lists one more granted scope.

## Capabilities

### New Capabilities
- `catalog`: the `album`, `playlist` and `artist` commands, their output shapes, paging, `--covers`, the "tracks not available" case and errors.

### Modified Capabilities
- `playback-state`: `play-uri` gains `--context <uri>`; its requirement and scenarios change.
- `auth`: `login` requests one more scope.

## Non-goals

- Artist top tracks, related artists, followers and popularity: Spotify removed them for apps in
  Development Mode (February 2026) and offers no replacement.
- Tracks of playlists the user neither owns nor collaborates on: not available to this app; no workaround
  (another app's `client_id` or Spotify's internal APIs) is attempted.
- An artist's `appears_on` and compilations: only albums and singles are listed.
- Editing playlists or albums, saving albums (`PUT /me/library` for albums), following artists.
- Podcast shows and their episodes as a screen (`show` URIs stay playable with `play-uri`).
- Playing from a position inside an artist or show context (Spotify's `offset` works for albums and
  playlists only).
- A `not_found` error code: a missing item is an `api_error` with the status, as for other commands.
- Any change to the plugin; the screens are a separate change in `noctalia-spotify`.

## Impact

- New `internal/api/catalog.go`, `internal/cli/catalog.go` and output types in `internal/output/model.go`;
  `internal/api/player.go` and `internal/cli/player.go` gain the context offset; `internal/auth/pkce.go`
  gains one scope; `internal/covers` is reused for `--covers`.
- `README.md` and `AGENTS.md` are updated in the same commit as the code.
- Requests: `GET /albums/{id}` and `GET /albums/{id}/tracks`, `GET /playlists/{id}` and
  `GET /playlists/{id}/items`, `GET /artists/{id}` and `GET /artists/{id}/albums`: a header request and a
  page request per call (one request when a playlist has no items to page).
- Risks: none of these endpoints has been called from this app in Development Mode, and the guide leaves
  the album and artist-album endpoints unconfirmed; the first task is a recorded check against a real
  account. The playlist `items` rename (`track` to `item`) is handled for both names. A collaborative
  playlist read with an old token may come back without items; the caller is told it is collaborative.

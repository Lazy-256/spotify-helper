## Context

See `proposal.md` for the motivation. The Noctalia plugin's next change (`add-detail-screens` in
`noctalia-spotify`) needs three things from the helper that it does not have: what an album, a playlist
and an artist contain, and "play from here". What the code and Spotify give us:

- Entry shapes already exist in `internal/output/model.go`: `Item` (track or episode), `AlbumEntry`,
  `ArtistEntry`, `PlaylistEntry`, with the embedded `Cover` and a `Covers()` method on results for the
  `--covers` post-processing. Paging is already specified for `playlists` and `albums` (`limit` 1–50,
  `offset`, echoed in the output).
- `internal/spotifyuri` parses URIs and `open.spotify.com` links into a type and an id.
- `PlayURI` sends `{"context_uri":…}` for contexts and `{"uris":[…]}` for single items.
- Spotify's February 2026 changes for Development Mode: `GET /artists/{id}/top-tracks` is gone with no
  replacement; `GET /playlists/{id}/tracks` became `/playlists/{id}/items` and "items are only returned for
  playlists the user owns or collaborates on" (otherwise metadata only, no items field); `GET /albums/{id}`
  and `GET /artists/{id}` remain; `/albums/{id}/tracks` and `/artists/{id}/albums` are not on the removed
  list but the guide does not confirm them. Nothing here has been called from this app yet.

## Goals / Non-Goals

**Goals:**
- Three read commands that need no new output vocabulary: every entry is one the plugin already knows.
- "Tracks not available" is a normal answer for a playlist, not an error, so a screen can still draw its
  header and a play button.
- Few requests: a header and a page.

**Non-Goals:**
- Anything the proposal lists as a non-goal; in particular no workaround for playlists the app is not
  allowed to read.

## Decisions

### Three commands, not one `browse`
`album`, `playlist` and `artist` each take their own kind of URI and have their own output shape (tracks
or albums as the list; a `tracks_available` flag and `collaborative` only for playlists). One `browse <uri>`
would return three shapes behind one name and push the dispatch into every caller. The singular `album`
next to the library's plural `albums` is a small naming wart; the argument (a URI) tells them apart and
the README says so. *Alternative:* `show <uri>` / `browse <uri>` — rejected as above.

### The header and the list are separate requests
Each command asks for the item itself (`GET /albums/{id}`, `/playlists/{id}`, `/artists/{id}`) and then
for one page of its list (`/albums/{id}/tracks`, `/playlists/{id}/items`, `/artists/{id}/albums`) with the
caller's `limit` and `offset`. *Alternative:* take the first page from the item's own response — rejected:
it works only for `offset` 0 and a fixed size, and the code would have two paths. Cost: one extra request;
the caller's own polling is slower than this by far.

### A playlist's availability is decided by Spotify's answer
`tracks_available` is true when the playlist response carries its items and false when it does not (the
Development Mode behavior) or when the items request is refused with 403 after the header succeeded; the
list request is then not sent at all in the first case. Any other failure of the items request is an
error as usual. This keeps the decision in one place and does not guess from the owner's name or the
user's id. `collaborative` comes from the playlist object so that a caller can tell "not allowed" from
"log in again": a collaborative playlist read with a token that predates `playlist-read-collaborative`
may come back without items.

### The scope is requested, not required
`login` asks for `playlist-read-collaborative`; no command calls `RequireScope` for it. Requiring it would
make `playlist` fail with `scopes_missing` for every playlist until the user logs in again, though nearly
all playlists need nothing new. `doctor` keeps flagging only `user-library-modify`. *Alternative:* require
it only for collaborative playlists — rejected, the header request would have to be made first and the
failure mode is Spotify's to define (see the spike).

### The playlist `items` shape is read tolerantly
After the February rename each entry holds its object under `item`, before it was `track`. The mapper reads
`item` and falls back to `track`, skips entries that are `null`, local files (`is_local` or no URI) and
types other than `track` and `episode`, and maps the rest to `Item`. Tracks from an album page are
simplified objects without an `album`, so the mapper fills `album` with the album being read, and takes
the cover from the album header, so `--covers` downloads one image for the whole album. A playlist's tracks
each have their own cover; `--covers` on a page of 50 can download up to 50 images (the cache and the
existing size limits apply) and callers should ask for fewer.

### "Play from here" is `play-uri <item> --context <uri>`
The play request gets `context_uri` (the album or playlist) and `offset: {uri: <item>}`. It is a flag of
the existing command rather than a new one: same device override, same error mapping, same output. Only
album and playlist contexts take an `offset` in Spotify's API, so artists and shows are refused with
`usage`. The helper does not check that the item belongs to the context; Spotify answers (and the shared
mapping reports) when it does not. *Alternative:* a separate `play-from` command — rejected, it would
duplicate the device handling.

### Spike first
Task 4 runs every new request once against the real account and keeps the answers as test fixtures
(identifiers and names replaced). The things to learn: do `/albums/{id}/tracks` and `/artists/{id}/albums`
answer; what `GET /playlists/{id}` returns for an own playlist, a followed one and a Spotify-made one (items
absent, or a 403, or something else); whether `total` is present when items are not; whether the
`fields` parameter can keep the header request small; and whether `/items` answers for the user's own
playlists. If an answer differs from the spec, the spec is updated before the code is finished.

## Risks / Trade-offs

- **An endpoint behaves differently in Development Mode than documented** → the spike runs before docs
  and the plugin change; the spec and the "tracks not available" rule are adjusted to what is found.
- **No tracks for most playlists the user finds in search** → by design, the header and `play-uri` still
  work; the plugin explains it. No workaround is attempted.
- **A page of 50 playlist tracks with `--covers` is slow on a cold cache** → the plugin chooses a smaller
  `limit`; the helper keeps its existing 2 MiB per file, 30 days and 100 MiB cache limits.
- **`play-uri --context` with an item that is not in the context** → Spotify's error is reported through
  the shared mapping; nothing is pre-checked.
- **Extra `login`** → only for collaborative playlists; everything else works with the old token.

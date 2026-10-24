## Context

See `proposal.md` for the motivation. What exists and what Spotify says:

- Entry shapes live in `internal/output/model.go` (`Item`, `ArtistEntry`, the embedded `Cover` and
  `Covers()` for the `--covers` post-processing). Paging flags and their echo (`limit`, `offset`, `total`)
  are specified for `playlists` and `albums`.
- `auth.Manager.RequireScope` already turns a token without a scope into `scopes_missing` (used by `save`
  and `unsave` for `user-library-modify`). `auth.Scopes` lists what `login` asks for.
- `GET /me/player/recently-played` (scope `user-read-recently-played`): `limit` 1–50 (default 20), the
  cursors `before` and `after` as Unix milliseconds that exclude each other, items `{track, played_at,
  context}`, `cursors` and `next` in the paging; no podcast episodes.
- `GET /me/top/{type}` (scope `user-top-read`): `type` `artists` or `tracks`, `time_range` `short_term`
  (about 4 weeks), `medium_term` (6 months, default) or `long_term` (a year), `limit` 1–50, `offset`; the
  fields `genres`, `followers`, `popularity` and the track `popularity` are deprecated.
- Neither page carries a Development Mode warning and neither is on the list of endpoints Spotify removed in
  February 2026. Neither has been called from this app yet.

## Goals / Non-Goals

**Goals:**
- Two small read commands in the existing vocabulary, so the plugin needs no new entry type.
- A missing scope is reported before any request, with the remedy ("log in again").

**Non-Goals:**
- Anything computed from the data; see the proposal.

## Decisions

### `recent` and `top`, with the type as an argument of `top`
`recent` is one command with one list. `top tracks` and `top artists` share the range, paging and
error handling and differ only in the entry type, which is also how Spotify names the endpoint, so they are
one command with a positional type and the output holds only the array of the requested type (the rule
`search --type` already follows). *Alternative:* `top-tracks` and `top-artists` — rejected, two commands
for one code path.

### The history keeps Spotify's cursors
Spotify pages the history by time, not by offset: the next older page is `before=<ms of the oldest item>`.
The helper passes `--before` and `--after` through unchanged and echoes the two cursors Spotify returns
(`null` when there are no more), so a caller pages with `recent --before <cursors.before>` and never has to
compute anything. A page of the plugin's size (around 10) is a `--limit`. *Alternative:* convert to
`offset` by re-reading from the top — rejected, it costs more requests and breaks when a new play arrives
between pages.

### The wrapper entry is nested, not flattened
Each history item is `{played_at, context, track}`, with `track` an ordinary `Item`. Flattening
`played_at` into the track would put a per-play field into a shape shared with search, queue and the
catalog commands. `context` is passed through as `{type, uri}` so a caller can offer "open the album this
was played from" later; it is `null` when absent.

### Time ranges are short names
`--range short|medium|long` instead of Spotify's `short_term` and friends: shorter to type, and the
output echoes the same short word so a screen can use it as a label key. The mapping is a single table.

### These two commands require their scopes
Unlike `playlist-read-collaborative` (requested, never required, because nearly every playlist needs
nothing new), `recent` and `top` do nothing without their scope, and Spotify's 403 would arrive as a vague
`api_error`. So each calls `RequireScope` first and fails with `scopes_missing` before any request. The
scopes are requested by `login` through an added requirement in the `auth` spec rather than by editing the
existing "Login runs the PKCE authorization flow" requirement, because `add-catalog-browse` edits that
requirement too and two edits of one requirement cannot both be archived. The cost is that the scope list
is described in two places in the main spec.

### Covers
Both commands pass their entries to the existing `--covers` post-processing. A page of 20 tracks of
different albums can download up to 20 images on a cold cache; the plugin chooses the `limit`.

## Findings of the live check (2026-10-24, real account, Development Mode)

- After `login` with the two new scopes everything answered as specified; nothing in the spec had to change.
- `recent`: `--limit 5` and `50` work (the same track can repeat: 38 distinct tracks in 50 plays); the entry keys
  are `played_at`, `context`, `track`; `context` is `null` for plays outside a context and `{type, uri}` (type
  `artist` here) otherwise. `cursors.before` pages to strictly older plays; `--after` the newest cursor gives 0 plays
  and both cursors `null`.
- `top tracks` and `top artists`: `--limit 50` works for both, and so does any `--offset` up to `total` (2039 tracks
  for the medium range, 547 for short; 164 artists for long, 140 for medium), so the lists are long, not capped at 50.
- Without the scopes the helper answered `scopes_missing` ("run `spotify-helper login` again") before any request.

## Risks / Trade-offs

- **Development Mode treats an endpoint differently than documented** → task 4.1 runs both once on the real
  account and keeps the answers as fixtures; the spec is adjusted before the code is finished.
- **Repeats in the history** → shown as returned; a caller that wants unique tracks removes them itself.
- **An old token** → `scopes_missing` with a clear message; the plugin shows "run `spotify-helper login`
  again", as it already does for saving.
- **Empty lists for a new account or a private session** → success with an empty array, never an error.
- **Two changes edit the scope list** → handled by the added requirement; after both are archived the main
  `auth` spec should be tidied so that one requirement lists every scope (a docs-only edit).

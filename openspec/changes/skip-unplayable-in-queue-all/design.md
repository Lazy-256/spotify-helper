## Context

See `proposal.md` for the motivation. Current state:

- `Client.QueueAddAll` reads each page through `Client.Album` / `Client.Playlist` (via `containerPage`) and
  counts `skipped` as the page size minus the mapped items. Those two methods also serve the `album` and
  `playlist` commands, whose requests and output must not change.
- Spotify fills `is_playable` (and `restrictions.reason`) on a track only when the request carries a `market`
  parameter. Checked on 2026-10-31 for `GET /tracks/{id}` and `GET /albums/{id}/tracks` with
  `market=from_token`: the unplayable track had `is_playable: false`, `restrictions.reason: "explicit"`, the
  others `true`. Playlist items were not checked yet.
- Spotify accepts such a track with 204 on `POST /me/player/queue`; the device drops it silently.

## Goals / Non-Goals

**Goals:**
- `queue-add-all` does not send tracks that Spotify marks as not playable and reports them in `skipped`.
- No change to the output of any other command, and no extra request.

**Non-Goals:** see the proposal.

## Decisions

### Filter in the API layer, not in `QueueAddAll`
`Item` (the output type) has no `is_playable` and the proposal keeps it that way, so the mark has to be read
while the wire object is still at hand. `wireItem` gains `IsPlayable *bool`. `Album` and `Playlist` keep their
signatures and become thin callers of unexported variants that take a `market` string; with a market they drop
entries whose `IsPlayable` is a non-nil `false`, the same place that already drops nulls and local files. Then
`QueueAddAll`'s existing arithmetic (`pageLen - len(items)`) counts them as skipped without any new counter.
*Alternative:* add `IsPlayable` to `Item` and filter in `QueueAddAll`: rejected, it widens the JSON contract
with the plugin for every command that returns items. *Alternative:* a `Market`/`PlayableOnly` field on
`Client`: rejected, it would change `album` and `playlist` for everyone by accident.

### `market=from_token`, only for `queue-add-all`
`from_token` makes Spotify use the country of the logged-in user, so no country code has to be configured or
stored. `album` and `playlist` stay without a market: adding one would change their answers (relinking, hidden
tracks) for a reason that has nothing to do with them.

### Absent mark means playable
Spotify omits `is_playable` for some answers (no market, episodes, older shapes). `*bool` distinguishes "false"
from "absent"; only an explicit `false` skips. This keeps every existing fixture and the Development Mode
behaviour working, and an unexpected answer degrades to today's behaviour instead of queueing nothing.

### Relinked tracks are queued under the URI Spotify returns
With a market Spotify may answer a track that is not available in the market by the playable copy of the
recording (a different `uri`, the asked one under `linked_from`) and then reports it playable. The helper queues
the returned `uri`, which is what Spotify says plays. `linked_from` is not read.

### The cap counts queued items, as before
Unplayable entries are removed before the cap is applied, exactly like local files, so they never use it up;
`truncated` keeps its meaning.

## Risks / Trade-offs

- **Playlists may behave differently from albums.** `/playlists/{id}/items` takes `market` and puts the mark on
  the nested track, but it is unverified. The live check covers it; if the mark is missing there, the playlist
  case keeps today's behaviour (absent means playable) and the design is updated before going on.
- **`skipped` no longer separates local files from unplayable tracks.** Accepted, the caller only shows a
  count; a reason can be a later addition.
- **Market and relinking change which URI is queued** for a few catalog tracks. Queueing the returned URI is
  what the Spotify clients do.
- **Account setting changes the numbers.** The same album gives different `added` before and after the user
  switches off the explicit filter. That is the point of the change and is documented.

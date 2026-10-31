## Why

A live check on 2026-10-31 showed that `queue-add-all` reports `added` 7 for a 7-track album while only 6 tracks
reach the queue: one track has `is_playable: false` (`restrictions.reason: "explicit"`, the account filters
explicit content). Spotify answers 204 to the add-to-queue request for such a track, and the device then drops
it silently, so the helper's count is wrong and the caller cannot tell. Spotify marks these tracks in the page it
already reads, so the helper can leave them out and count them as skipped.

## What Changes

- `queue-add-all` asks Spotify for the user's market (`market=from_token`) when it reads album tracks and playlist
  items, which makes Spotify fill in `is_playable` for each track, and does not queue entries with
  `is_playable: false`.
- Such entries are counted in `skipped`, like local files; they do not count against the cap of 100.
- Nothing else changes: `album` and `playlist` keep their requests and output, `queue-add` still sends whatever
  it is given.

**Does not break the JSON contract**: the output of `queue-add-all` keeps its fields; only the numbers differ
(`added` is lower, `skipped` higher) for containers that hold unplayable tracks.

## Capabilities

### New Capabilities

None.

### Modified Capabilities
- `queue`: the requirement "Add a whole album or playlist to the queue" defines `skipped` to include entries
  that Spotify marks as not playable for the user, and says they are not sent.

## Non-goals

- Showing `is_playable` in `album`, `playlist`, `queue` or any other output, or adding a field to `Item`.
- Warning the caller about *why* an entry was skipped (explicit filter, region): `skipped` stays one number.
- Changing the explicit-content setting or detecting it from the account.
- Making `queue-add` check a single track: it takes one URI and no market lookup, and the plugin does not need it.
- Checking the queue after the run for items that Spotify dropped for other reasons.

## Impact

- `internal/api`: `wireItem` reads `is_playable`; `Album` and `Playlist` get an internal variant with a market
  and a playable-only filter used by `QueueAddAll`; their public signatures and requests stay as they are.
- Requests for `queue-add-all` gain one query parameter (`market=from_token`) on the tracks/items read; the
  number of requests is unchanged.
- Risk: with a market, Spotify may *relink* a track (answer with another URI of the same recording and put the
  original under `linked_from`). The helper queues the URI it is given, which is the playable one. A live check
  confirms this and that playlists honour the parameter the same way albums do.
- `README.md` and `AGENTS.md` are updated in the same commit as the code.

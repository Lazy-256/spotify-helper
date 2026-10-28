## Why

The plugin wants an "Add to queue" button for a whole album or playlist, and for the rest of one from a chosen
track. Spotify's `POST /me/player/queue` takes exactly one track or episode per request, so the helper has to
walk the container and queue its items one by one, in order, in a single process (one token read, one
cooldown check) instead of the plugin starting the helper dozens of times.

## What Changes

- New command `queue-add-all <album|playlist uri|link> [--offset N] [--device ID]`: reads the container in
  pages of 50 and queues its tracks and episodes sequentially, in play order, starting at position `--offset`
  (default 0), **at most 100 items per run**.
- Output `{"ok":true,"uri":…,"added":N,"skipped":S,"total":T,"offset":K,"truncated":bool}`: `truncated` is true
  when the cap stopped the run before the end, so the caller can say "Added 100 of 342".
- A failure after some items were queued (429, network, a refused request) still fails the command, as every
  failure does, and carries `error.added` (what is already in the queue; Spotify has no way to take it back) and,
  when known, `error.total`.
- New error code `tracks_unavailable` (exit 1): the playlist's items are not served to this app (someone else's
  playlist, Development Mode), nothing was queued.
- Artist, track and episode URIs are rejected with `usage`; `queue-add` stays as it is.

**Does not break the JSON contract**: a new command, a new error code and two optional fields of the error
object (`added`, `total`) that only this command sets.

## Capabilities

### New Capabilities

None.

### Modified Capabilities
- `queue`: an added requirement for `queue-add-all` (arguments, order, cap, output, partial failure, no item list).
- `output-contract`: the error-code table gains `tracks_unavailable`.

## Non-goals

- Removing or reordering what was queued: Spotify offers no such call (see "No queue editing beyond adding").
- Queueing an artist's whole catalog, or several containers in one call.
- A `--limit` flag: the cap of 100 is a fixed safeguard, not a setting.
- Pacing or automatic retries: a 429 stops the run and is reported (see the live check and its risk).
- Making `queue-add` accept containers.

## Impact

- New `internal/api` method (`QueueAddAll`, built on `Album`/`Playlist` page reads and `QueueAdd`), a command in
  `internal/cli/queue.go`, optional `Added`/`Total` fields on `output.Error`, the new error code and its exit
  code in `internal/output`.
- `README.md` and `AGENTS.md` are updated in the same commit as the code.
- Requests: up to 2 page reads plus up to 100 queue POSTs per call, strictly sequential.
- Risks: 100 POSTs in a row may hit Spotify's rolling rate limit (a recorded live check measures it); the plugin's
  row index equals Spotify's playlist position only when the playlist has no local files before that row, because
  the helper does not list local files (documented in the design).

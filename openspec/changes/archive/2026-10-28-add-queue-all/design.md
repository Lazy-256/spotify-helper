## Context

See `proposal.md` for the motivation. What exists and what a live spike (2026-10-28, real account) showed:

- `Client.QueueAdd` posts one URI to `/me/player/queue`; `Client.Album` and `Client.Playlist` read one page
  (`limit` 1–50, `offset`) and already map tracks, drop `null` and local entries of playlists, and report
  `total`; `Playlist` returns `TracksAvailable=false` when the items are not served.
- Two `queue-add` calls in a row queue their items in the order given (A, then B), after whatever the user
  queue already held and before the items of the playing context. So a sequential loop in container order
  produces the container's order in the queue.
- Errors leave the program as one `*output.Error` printed by `cli.Run`; a 429 is recorded on disk by the gate
  and fails the later calls at once. `Error` has `RetryAfter` as its one optional extra field.
- The Web API has no call that removes a queue item.

## Goals / Non-Goals

**Goals:**
- One process queues a container in order, with a cap that makes a mistaken press harmless.
- The caller learns exactly how much was queued, also when the run fails half-way.

**Non-Goals:** see the proposal.

## Decisions

### The loop lives in the helper, as one new command
The plugin only needs to send one command and parse one answer; the token and cooldown work happens once. A
plugin loop would start the helper per track (about 300 ms of overhead each) and would have to interpret dozens
of errors. *Alternative:* extend `queue-add` to accept containers — rejected: `queue-add`'s "one item, URI
checked" contract is specified and tested, and a command that sometimes sends one request and sometimes a
hundred is harder to reason about.

### Sequential, no pacing, stop on the first failure
Order matters and parallel posts would shuffle it, so the loop is a plain `for`. The helper does not sleep
between posts and does not retry: a 429 ends the run through the existing gate, the answer says how many items
are in the queue and how long to wait. If the live check shows that 100 posts regularly hit the limit, pacing
becomes a follow-up with a measured delay, not a guess now.

### Cap of 100 added items, pages of 50, no flag
The cap counts queued items, not examined entries, so skipped entries do not eat it. Pages are read lazily: a
second page only when the cap is not yet reached and entries remain. The cap is a constant (`MaxQueueAll`), not
a flag, because its job is to protect the queue and the rate budget from a mistake; the caller shows
`truncated`. *Alternative:* `--limit`: rejected as an invitation to defeat the safeguard, and a caller that
wants more can call again with `--offset`.

### `--offset` is Spotify's position, the same one `album` and `playlist` use
The plugin starts a run at a chosen track by passing the position of that row (its page `offset` plus its index).
For an album that is exact. For a playlist the helper's `playlist` command does not list local files, so a row's
index can be smaller than Spotify's position when a local file comes before it; the run then starts a few items
early. Local files are rare, cannot be played through the Web API, and the effect is "queued one or two extra
tracks", so the limitation is documented and not worked around. *Alternative:* `--from <uri>` matching the first
occurrence: rejected, because playlists may hold the same track twice, and that is exactly the case an index
handles correctly.

### Failure part-way: the error object carries `added` and `total`
Every failure of the helper is one error on stderr with a non-zero exit, and that stays true. The two optional
fields (`added`, `total`) on `output.Error` are the same kind of extension as `retry_after`: omitted unless set,
set only by this command, callers ignore fields they do not know. *Alternative:* exit 0 with `{"ok":false,…}`
on stdout — rejected, it would make this the only command where a failure is a success exit code.

### A new error code for unreadable playlists
The plugin has to tell "nothing to queue, Spotify will not serve this playlist" from a transient `api_error`
and show a fixed text for it. The code `tracks_unavailable` takes exit code 1 (it falls under the default of
`ExitCode`), so no exit-code table row is added, only the code. The detail screen already knows the same fact
from `tracks_available`; the code serves the search rows, where the plugin has not read the playlist.

### Skipped entries are counted, not hidden
Local files and entries without a URI cannot be queued. They are counted in `skipped` so that
`added + skipped` accounts for the examined range, which the plugin can use for an honest message.

## Live check findings (2026-10-28, real account)

- An album of 24 tracks was queued in 4.6 s (about 190 ms per item), in album order; `--offset 3` queued 21 in 3.9 s.
- Two runs of 100 (a 264-item playlist, a minute apart) completed with no 429. A run of 79 posts in 16.6 s that
  came right after 45 posts of the previous minute was rate limited (`retry_after` 30) and reported `added` 79,
  `total` 110: the limit is a rolling window that adds up close runs. No pacing was added.
- Someone else's playlist answered `tracks_unavailable` in 0.24 s with no post; an editorial playlist (`37i9dQZ…`)
  answered `api_error` with HTTP 404, like `playlist`. The plugin has to say both kindly (see its change).
- Not covered: a playlist with a local file (the account has none checked), so `skipped` for local files is
  tested only with `httptest`.

## Risks / Trade-offs

- **100 POSTs may trip the rate limit.** The first live check queues a 100-item run and records the duration
  and whether 429 appeared; the result goes into this design and into `AGENTS.md`.
- **Pollution of the queue.** A mistaken press queues up to 100 items that can only be removed one by one.
  Mitigated by the cap and by the plugin's skip-list removal; no confirmation step (decided with the user).
- **The queue view shows 20 items.** After a long run the user sees only the first 20; this is Spotify's view,
  already documented for `queue`.
- **Time.** A 100-item run takes a number of seconds; the plugin runs it outside its command queue (a plugin
  change) so playback controls keep working.

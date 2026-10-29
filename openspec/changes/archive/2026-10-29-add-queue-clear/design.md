## Context

See `proposal.md` for the motivation. What exists and what the live spike (2026-10-28, real account) showed:

- `Client.State`, `Client.PlayURI`, `Client.Queue` (up to 20 entries, the user's entries first), `Client.Next` and
  `Client.Pause` exist, each taking a device id; every failure is one `*output.Error`, a 429 records a cooldown.
- `play-uri` of the playing track made `state.context` `null` and left the queue unchanged: the entries added by
  hand survive a change of context, and the rest of the old context is dropped.
- `next` while paused started playback of the next entry and consumed the head of the queue.
- `queue` merges the user's entries and the context's, with no marker between them, and shows at most 20. After
  the context has been dropped every entry shown is the user's.
- Close runs add up in Spotify's rate window (the `queue-add-all` live check): 100 requests on a clean window
  passed twice, ~125 within a minute did not.

## Live check findings (2026-10-29, real account)

- A first run was cut off after exactly 30 s with `network_error` and `skipped` 71: `opTimeout`, the 30 s budget of
  one whole command, is too short for 100 sequential `next` (about 0.42 s each, 45 s in all; a `queue-add` takes
  0.19 s). The error carried the partial count, as designed.
- A second run sent 26 `next` and failed with HTTP 403 "Player command failed: Restriction violated". Afterwards
  `queue` still showed ten entries, all with the URI of the item playing now (`repeat` off): once the queued
  entries are used up and the context is a single item, Spotify pads the queue with copies of the playing item and
  refuses `next` for them. "The queue shows nothing" never became true, so the loop ran into the refusal.

- A run that sent 99 `next` and then failed with 403 while the queue still held tracks was not at the end: three
  manual `next` right afterwards were accepted. Spotify refuses a command for a moment while the device changes
  track, and at 0.4 s per `next` one of a hundred lands in such a moment.
- A third run (right after 100 `queue-add`) was rate limited after 18 `next`, as expected for close runs; the fourth
  failed with HTTP 403 after 77 more. Afterwards `queue` had no current item and nothing was playing, and both
  `pause` and `next` answered the same 403: a `next` had been sent past the end of the queue and stopped the player.
  Most likely the device applied the last `next` calls late, so the queue read after a batch still showed entries
  that were already used up and the loop counted from it.

- The final run, with the fixes below, queued and cleared a real queue: `{"ok":true,"skipped":96,"empty":true}` in
  48 s (about 0.5 s per `next`), playback paused at position 0 with no context, and the queue then showed only ten
  copies of the playing item (padding, not counted). Nothing differed from the spec after the three fixes.

## Goals / Non-Goals

**Goals:**
- One command that gets rid of everything in front of the current item, as the user asked, including the rest of
  the playlist.
- A partial result that says how far it got, so the caller can repeat.

**Non-Goals:** see the proposal.

## Decisions

### Drop the context first, then play through what is left
Dropping the context is one request and removes an unbounded number of context entries (a 500-track playlist),
which could not be skipped one by one within the cap. What remains is exactly the user's entries, and only those
need `next`. *Alternative:* `next` through everything: rejected, it would walk the whole playlist.

### Restart the current item rather than a stub
`play-uri` of the current item is the only way found to replace the context without changing what plays. The
item restarts from the beginning (documented). *Alternative:* restore the position with `seek` afterwards:
one more request, and the run ends paused anyway; not done.

### Batches of what the queue shows
`queue` returns at most 20 entries, so the loop reads, sends up to 20 `next`, reads again. It never sends more
`next` than entries it saw, so a queue that is already empty costs no `next`. The cap of 100 `next` per run keeps
a run inside the rate window that 100 requests passed on a clean window; a caller repeats for the rest.

### Copies of the playing item are not entries
The loop ends when the queue holds no *real* entry: a trailing run of entries with the URI of the item playing
now is dropped before counting (`withoutPadding`). *Alternative:* treat the 403 as the end of the queue: rejected,
it hides real failures (a restricted device) behind a success. *Trade-off:* a queued entry that is the same item as
the current one and stands last is taken for padding and stays queued; for a button that empties everything this
is accepted and documented.

### Settled reads, and a refusal is judged by the queue
After a batch of `next` the queue is read until two reads in a row agree (at most four more reads, 250 ms apart),
so the next batch is counted from the truth. When `next` is refused with HTTP 403, the queue is settled once more:
if it holds no real entry the run ended normally (Spotify refuses a `next` with nothing to skip to), otherwise the
refusal is a real failure and is reported with `skipped`. A refused final pause is ignored: with no current item
nothing is playing. With no current item nothing counts as a real entry either. *Alternative:* a fixed pause after
every batch: rejected, it is a guess about the device's speed.

### A refusal while entries remain is tried again
A `next` refused with 403 while the settled queue still holds real entries is sent again, up to three more times.
This is the one retry in the helper; rate limits (429) are still never retried. *Alternative:* a fixed pause
between all `next` requests: rejected, it slows every run for the sake of one in a hundred.

### A longer budget for the bulk commands
`queue-clear` and `queue-add-all` run under 120 s for the whole command (`bulkTimeout`) instead of 30 s; every
request keeps its own 10 s limit. The ordinary commands keep 30 s. *Alternative:* a cap of 50: rejected, a long
queue would need many presses.

### Always pause at the end
Each `next` starts an entry, so after the loop an arbitrary entry plays. A run therefore ends with `pause`, also
when it stopped at the cap, and the caller shows that it is not finished. The pause is not attempted after a rate
limit or a network error, because it would fail the same way.

### Failure part-way: `error.skipped`, as `queue-add-all` has `added`
Same shape as the `queue-add-all` change: the failure stays a failure (non-zero exit, one error object); the
optional integer `skipped` tells how many entries were already advanced past, set only by this command.

## Risks / Trade-offs

- **Padding of the queue** (found live, see above) is handled by `withoutPadding`. If Spotify also offers suggested
  entries other than copies of the playing item (autoplay), the loop would skip them until the cap; the next
  live check looks at `queue` after a full clear.
- **Audible blips.** Every entry starts for a moment, also while paused (verified). Unavoidable; the plugin warns.
- **Restart of the current item** and loss of the old context are the point of the command and cannot be undone.
- **Rate window.** A clear right after many other requests may stop early with `rate_limited` and `skipped`; the
  caller waits `retry_after` and runs it again.

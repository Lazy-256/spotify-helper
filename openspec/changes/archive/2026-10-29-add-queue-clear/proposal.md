## Why

The plugin wants a "Clear queue" button. Spotify's Web API cannot remove queue entries or clear the queue, and a
queue that holds hundreds of tracks (after a few `queue-add-all` runs) can only be emptied by playing through it.
A live spike (2026-10-28, real account) showed two things that make an emulation possible:

- Starting the playing item again with `play-uri` makes the playback context `None`: the rest of the playlist
  that was playing is gone, while the entries that were added to the queue stay, in order.
- `next` consumes the head of the queue, also when playback is paused (it then starts playing the next item).

## What Changes

- New command `queue-clear [--device ID]`: it drops the playback context (restarts the current item as its own
  context), then advances through the queue with `next`, one request per entry, until `queue` shows nothing or
  100 `next` requests were sent, and finally pauses playback.
- Output `{"ok":true,"skipped":N,"empty":bool}`: `skipped` is the number of entries advanced past, `empty` is true
  when the queue read at the end showed nothing. When `empty` is false the caller can run the command again.
- A failure after the context was dropped (429, network, a refused request) fails the command as every
  failure does and carries `error.skipped` (what was already advanced past).
- The spec "No queue editing beyond adding" is reworded: single entries still cannot be removed or reordered;
  `queue-clear` is an emulation that advances through the queue.

**Does not break the JSON contract**: a new command and one optional field of the error object (`skipped`) that
only this command sets.

## Capabilities

### New Capabilities

None.

### Modified Capabilities
- `queue`: an added requirement for `queue-clear`; the requirement "No queue editing beyond adding" is modified.

## Non-goals

- Removing or reordering single entries (still impossible).
- Keeping the current item's position: it restarts from the beginning, and that is a documented side effect.
- Undoing a clear, or restoring the old context.
- Silence while clearing: every `next` starts the next entry on the device for a moment, also when paused.
- Clearing only the entries added by hand and keeping the rest of the playlist (Spotify's own button): the
  Web API does not tell them apart, and the user asked for everything to go.
- Pacing between requests (see the risks; a rate-limited run reports what it did and can be repeated).

## Impact

- New `internal/api` method (`QueueClear`, built on `State`, `PlayURI`, `Queue`, `Next`, `Pause`), a command in
  `internal/cli/queue.go`, an optional `Skipped` field on `output.Error`.
- `README.md` and `AGENTS.md` are updated in the same commit as the code.
- Requests: one state read, one `play-uri`, up to 6 queue reads, up to 100 `next`, one `pause`, all sequential.
- Risks: with the queue empty Spotify may fill `queue` with autoplay suggestions, which would keep the loop going
  until the cap (checked in the live check first); 100 `next` in a row may be rate limited like 100 posts were when
  runs came close together (reported as a partial result); an episode or a track without a `uri` cannot be
  restarted (the command then fails with `no_active_device`, which is what nothing playing means here).

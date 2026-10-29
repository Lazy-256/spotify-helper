## ADDED Requirements

### Requirement: Clear the queue
`spotify-helper queue-clear [--device ID]` SHALL empty the playback queue of the active device (or of `--device <id>` when given) as far as the Web API allows, by playing through it, in this order and without parallel requests:
1. Read the playback state. With no active device or no current item it SHALL fail with `no_active_device` before changing anything.
2. Start the current track or episode again as its own context (the effect of `play-uri <current item uri>`), which drops the rest of the playback context; the item restarts from the beginning.
3. Repeat: read the queue; when it holds no real entry, stop; otherwise send one `next` per real entry shown, in order, then read the queue again until two reads in a row agree (a device applies `next` late, and counting from a stale read would send a `next` past the end). At most 100 `next` requests SHALL be sent per run. When no item is current nothing counts as real. Entries at the end of the queue that have the URI of the item playing now SHALL NOT count as real, because Spotify shows copies of the playing item behind the queued entries once the context is a single item and the queued entries are used up, and refuses `next` for them.
4. Pause playback, whatever ended the loop; a pause that Spotify refuses with HTTP 403 (no item is current, so nothing plays) SHALL NOT fail the command.

It SHALL output `{"ok":true,"skipped":<n>,"empty":<bool>}` where `skipped` is the number of `next` requests sent and `empty` is true exactly when the last queue read showed no real entry. A run that stopped at the cap with entries left SHALL succeed with `empty` false.

#### Scenario: A queue of three
- **WHEN** the context is a playlist, three entries are queued and the user runs `queue-clear`
- **THEN** the current item is started again as its own context, three `next` requests are sent, playback is paused last, and the output is `{"ok":true,"skipped":3,"empty":true}`

#### Scenario: An empty queue
- **WHEN** nothing is queued
- **THEN** the context is dropped, no `next` is sent, playback is paused, and the output has `skipped` 0 and `empty` true

#### Scenario: Copies of the playing item
- **WHEN** three entries are queued and, behind them, Spotify shows ten copies of the item playing now (and answers `next` for those copies with HTTP 403 "Restriction violated")
- **THEN** exactly three `next` requests are sent, playback is paused last, and the output is `{"ok":true,"skipped":3,"empty":true}`

#### Scenario: A late device
- **WHEN** the first queue read after a batch of `next` still shows an entry that was just used up
- **THEN** no further `next` is sent for it, the run ends with `empty` true, and the player still has a current item

#### Scenario: Refused next at the end
- **WHEN** Spotify answers a `next` with HTTP 403 and the queue read afterwards holds no real entry
- **THEN** the command succeeds with `empty` true, the refused request not counted in `skipped`

#### Scenario: A refusal that passes
- **WHEN** Spotify answers one `next` with HTTP 403 while the queue still holds real entries, and the same request succeeds when it is sent again
- **THEN** the run goes on and the refused attempt is not counted in `skipped`

#### Scenario: Refused next with entries left
- **WHEN** Spotify keeps answering a `next` with HTTP 403 (four attempts in all) while the queue still holds real entries
- **THEN** the command fails with the error of that response and `error.skipped`

#### Scenario: Pause refused
- **WHEN** the final pause is answered with HTTP 403
- **THEN** the command still succeeds

#### Scenario: Only copies of the playing item
- **WHEN** the queue shows nothing but copies of the item playing now
- **THEN** no `next` is sent and the output has `skipped` 0 and `empty` true

#### Scenario: More than the cap
- **WHEN** 250 entries are queued (the queue shows 20 at a time)
- **THEN** exactly 100 `next` requests are sent in batches of at most 20 with a queue read between them, playback is paused, and the output has `skipped` 100 and `empty` false

#### Scenario: Nothing playing
- **WHEN** Spotify reports no active device, or no current item
- **THEN** the command exits 4 with `error.code` `no_active_device`, no `play-uri`, `next` or pause is sent, and the error has no `skipped`

#### Scenario: Another device
- **WHEN** the user runs `queue-clear --device D1`
- **THEN** every request that supports a device id carries `D1`

### Requirement: A failure part-way reports what was skipped
A `next` refused with HTTP 403 while the queue still holds real entries SHALL be sent again up to three more times, after the queue has settled, before it counts as a failure (Spotify also refuses commands for a moment while a device changes track). When a request fails after the context was dropped, `queue-clear` SHALL fail with the error of that failure (`rate_limited` with `retry_after`, `network_error`, `api_error`, `premium_required`) and SHALL add the integer field `error.skipped` with the number of `next` requests already sent, omitted when it is 0. The failure SHALL stop the run at once and nothing SHALL be retried; the final pause is attempted only when the failure was not a rate limit or a network error. A cooldown recorded by an earlier 429 SHALL make the command fail with `rate_limited` before any request.

#### Scenario: Rate limited in the middle
- **WHEN** Spotify answers 429 with `Retry-After: 12` to the 31st `next`
- **THEN** the command exits 5 with `error.code` `rate_limited`, `error.retry_after` 12 and `error.skipped` 30, and no further request is sent

#### Scenario: Cooldown already active
- **WHEN** a rate-limit cooldown is recorded
- **THEN** the command exits 5 at once without any request to Spotify

## MODIFIED Requirements

### Requirement: No queue editing beyond adding
The helper SHALL NOT provide commands to remove or reorder single queue entries, because the Spotify Web API offers no such operation. Emptying the queue is possible only as `queue-clear`, which plays through the queue instead of editing it.

#### Scenario: Unsupported command
- **WHEN** the user runs `spotify-helper queue-remove`
- **THEN** the command exits 2 with `error.code` `usage`

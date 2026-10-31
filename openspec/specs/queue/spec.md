# queue Specification

## Purpose
Lets a caller see what Spotify will play next and append a track or episode to the playback queue of the user's Connect device.

## Requirements

### Requirement: Read the queue
`spotify-helper queue` SHALL output `{"now_playing":<item|null>,"queue":[<item>,…]}` where each `<item>` has the same shape as `state`'s `item` (`type`, `uri`, `id`, `name`, `artists`, `album`, `duration_ms`, `cover_url`). The `queue` array SHALL hold the upcoming items in play order exactly as Spotify returns them, which can be an incomplete view of long contexts.

#### Scenario: Items queued
- **WHEN** a track is playing and two items are upcoming
- **THEN** `now_playing` is that track and `queue` has the two items in order

#### Scenario: Nothing playing
- **WHEN** Spotify returns no current item and an empty queue
- **THEN** the output is `{"now_playing":null,"queue":[]}` and the exit code is 0

### Requirement: Add to the queue
`spotify-helper queue-add <uri>` SHALL append a single item to the queue of the active device (or of `--device <id>` when given) and output `{"ok":true,"uri":"<normalized-uri>"}`. Only `spotify:track:` and `spotify:episode:` URIs, or the equivalent `https://open.spotify.com/track/<id>` and `/episode/<id>` links, SHALL be accepted; any other value, including album, playlist and artist URIs, SHALL fail with `usage`. A web link SHALL be normalized to its URI in the output.

#### Scenario: Queue a track
- **WHEN** the user runs `queue-add spotify:track:abc` while playing
- **THEN** Spotify receives the add-to-queue request and the output is `{"ok":true,"uri":"spotify:track:abc"}`

#### Scenario: Queue from a web link
- **WHEN** the user runs `queue-add https://open.spotify.com/track/abc?si=xyz`
- **THEN** the output `uri` is `spotify:track:abc`

#### Scenario: Album is rejected
- **WHEN** the user runs `queue-add spotify:album:abc`
- **THEN** the command exits 2 with `error.code` `usage` and nothing is sent to Spotify

#### Scenario: No active device
- **WHEN** Spotify reports no active device and no `--device` is given
- **THEN** the command exits 4 with `error.code` `no_active_device`

### Requirement: No queue editing beyond adding
The helper SHALL NOT provide commands to remove or reorder single queue entries, because the Spotify Web API offers no such operation. Emptying the queue is possible only as `queue-clear`, which plays through the queue instead of editing it.

#### Scenario: Unsupported command
- **WHEN** the user runs `spotify-helper queue-remove`
- **THEN** the command exits 2 with `error.code` `usage`

### Requirement: Add a whole album or playlist to the queue
`spotify-helper queue-add-all <uri>` SHALL append the tracks and episodes of an album or a playlist to the queue of the active device (or of `--device <id>` when given), one request per item, strictly in the order of the container and without parallel requests. Only `spotify:album:` and `spotify:playlist:` URIs, or the equivalent `https://open.spotify.com/album/<id>` and `/playlist/<id>` links, SHALL be accepted; any other value, including track, episode and artist URIs, SHALL fail with `usage` before anything is sent. `--offset <n>` (an integer ≥ 0, default 0, otherwise `usage`) SHALL start at that position of the container as Spotify numbers it. The command SHALL read the container for the user's market, so that Spotify says which tracks are playable, and SHALL NOT send a track that Spotify marks as not playable; a track without that mark SHALL be treated as playable. The command SHALL read the container in pages of 50 and SHALL queue at most 100 items per run. It SHALL output `{"ok":true,"uri":"<normalized-uri>","added":<n>,"skipped":<n>,"total":<n>,"offset":<n>,"truncated":<bool>}` where `total` is the number of items Spotify reports for the container, `added` the number queued, `skipped` the number of examined entries that cannot be queued (local files, removed or unavailable entries without a URI, and tracks that Spotify marks as not playable for the user, for example explicit tracks when the account filters explicit content), and `truncated` is true exactly when the run stopped at the cap with entries left unexamined. A container with nothing to queue from `offset` on SHALL succeed with `added` 0.

#### Scenario: A whole album
- **WHEN** the user runs `queue-add-all spotify:album:abc` for an album of 12 tracks while playing
- **THEN** Spotify receives 12 add-to-queue requests, one after another in track order, and the output has `added` 12, `skipped` 0, `total` 12, `offset` 0 and `truncated` false

#### Scenario: From a track on
- **WHEN** the user runs `queue-add-all spotify:album:abc --offset 4` for that album
- **THEN** the first request is for the fifth track, 8 requests are sent, and the output has `added` 8, `total` 12 and `offset` 4

#### Scenario: A playlist with more than the cap
- **WHEN** the playlist holds 342 items
- **THEN** exactly 100 items are queued in playlist order, the output has `added` 100, `total` 342 and `truncated` true, and the third page of the playlist is not read when the first two pages already filled the cap

#### Scenario: Local files and gaps
- **WHEN** 3 entries of the examined range are local files
- **THEN** they are not sent to Spotify and the output's `skipped` is 3

#### Scenario: Unplayable tracks
- **WHEN** 1 track of a 7-track album is marked as not playable for the user
- **THEN** 6 add-to-queue requests are sent, in album order, and the output has `added` 6, `skipped` 1 and `total` 7

#### Scenario: Unplayable tracks do not use up the cap
- **WHEN** a playlist of 130 items has 5 not playable tracks among its first 100 entries
- **THEN** exactly 100 playable items are queued, in playlist order, and `truncated` is true

#### Scenario: Tracks without a playable mark
- **WHEN** Spotify gives no playable mark for a track
- **THEN** the track is queued as before

#### Scenario: A web link
- **WHEN** the user runs `queue-add-all https://open.spotify.com/playlist/abc?si=xyz`
- **THEN** the output `uri` is `spotify:playlist:abc`

#### Scenario: Not a container
- **WHEN** the user runs `queue-add-all spotify:track:abc` or `queue-add-all spotify:artist:abc`
- **THEN** the command exits 2 with `error.code` `usage` and nothing is sent to Spotify

#### Scenario: Bad offset
- **WHEN** the user runs `queue-add-all spotify:album:abc --offset -1`
- **THEN** the command exits 2 with `error.code` `usage`

#### Scenario: Offset past the end
- **WHEN** `--offset` is larger than the container's `total`
- **THEN** the command succeeds with `added` 0, `total` unchanged and `truncated` false

#### Scenario: No active device
- **WHEN** Spotify answers the first add-to-queue request with "no active device" and no `--device` is given
- **THEN** the command exits 4 with `error.code` `no_active_device` and no `error.added`

### Requirement: Playlists whose items are not served
When the helper cannot read the items of a playlist (the playlist is not the user's own or collaborative, as in Development Mode, or Spotify refuses its items), `queue-add-all` SHALL fail with `error.code` `tracks_unavailable` before queueing anything.

#### Scenario: Someone else's playlist
- **WHEN** the user runs `queue-add-all` for a playlist that Spotify serves without items
- **THEN** the command exits 1 with `error.code` `tracks_unavailable` and no add-to-queue request is sent

### Requirement: A failure part-way reports what was queued
When a request fails after at least one item was queued, `queue-add-all` SHALL fail with the error of that failure (`rate_limited` with `retry_after`, `network_error`, `api_error`, `premium_required`) and SHALL add the integer field `error.added` with the number of items already queued; `error.total` SHALL be added too when the total of the container is known. The failure SHALL stop the run at once: no further item is sent and nothing is retried. Items already queued SHALL stay queued, because the Web API cannot remove them. A failure before the first item was queued SHALL carry neither field.

#### Scenario: Rate limited in the middle
- **WHEN** Spotify answers 429 with `Retry-After: 12` to the ninth add-to-queue request of a 30-track album
- **THEN** the command exits 5 with `error.code` `rate_limited`, `error.retry_after` 12, `error.added` 8 and `error.total` 30, and no tenth request is sent

#### Scenario: Failure before the first item
- **WHEN** reading the album fails with `api_error`
- **THEN** the error has neither `added` nor `total`

#### Scenario: Cooldown already active
- **WHEN** a rate-limit cooldown is still recorded from an earlier run
- **THEN** the command exits 5 at once without any request to Spotify and without `error.added`

### Requirement: Clear the queue
`spotify-helper queue-clear` SHALL empty the playback queue of the active device as far as the Web API allows, by playing through it, in this order and without parallel requests:
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

#### Scenario: No device flag
- **WHEN** the user runs `queue-clear --device D1`
- **THEN** the command exits 2 with `error.code` `usage` and nothing is sent to Spotify

### Requirement: A failure part-way reports what was skipped
A `next` refused with HTTP 403 while the queue still holds real entries SHALL be sent again up to three more times, after the queue has settled, before it counts as a failure (Spotify also refuses commands for a moment while a device changes track). When a request fails after the context was dropped, `queue-clear` SHALL fail with the error of that failure (`rate_limited` with `retry_after`, `network_error`, `api_error`, `premium_required`) and SHALL add the integer field `error.skipped` with the number of `next` requests already sent, omitted when it is 0. The failure SHALL stop the run at once and nothing SHALL be retried; the final pause is attempted only when the failure was not a rate limit or a network error. A cooldown recorded by an earlier 429 SHALL make the command fail with `rate_limited` before any request.

#### Scenario: Rate limited in the middle
- **WHEN** Spotify answers 429 with `Retry-After: 12` to the 31st `next`
- **THEN** the command exits 5 with `error.code` `rate_limited`, `error.retry_after` 12 and `error.skipped` 30, and no further request is sent

#### Scenario: Cooldown already active
- **WHEN** a rate-limit cooldown is recorded
- **THEN** the command exits 5 at once without any request to Spotify

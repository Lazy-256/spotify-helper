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
The helper SHALL NOT provide commands to remove or reorder queue entries, because the Spotify Web API offers no such operation.

#### Scenario: Unsupported command
- **WHEN** the user runs `spotify-helper queue-remove`
- **THEN** the command exits 2 with `error.code` `usage`

### Requirement: Add a whole album or playlist to the queue
`spotify-helper queue-add-all <uri>` SHALL append the tracks and episodes of an album or a playlist to the queue of the active device (or of `--device <id>` when given), one request per item, strictly in the order of the container and without parallel requests. Only `spotify:album:` and `spotify:playlist:` URIs, or the equivalent `https://open.spotify.com/album/<id>` and `/playlist/<id>` links, SHALL be accepted; any other value, including track, episode and artist URIs, SHALL fail with `usage` before anything is sent. `--offset <n>` (an integer ≥ 0, default 0, otherwise `usage`) SHALL start at that position of the container as Spotify numbers it. The command SHALL read the container in pages of 50 and SHALL queue at most 100 items per run. It SHALL output `{"ok":true,"uri":"<normalized-uri>","added":<n>,"skipped":<n>,"total":<n>,"offset":<n>,"truncated":<bool>}` where `total` is the number of items Spotify reports for the container, `added` the number queued, `skipped` the number of examined entries that cannot be queued (local files, removed or unavailable entries without a URI), and `truncated` is true exactly when the run stopped at the cap with entries left unexamined. A container with nothing to queue from `offset` on SHALL succeed with `added` 0.

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

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

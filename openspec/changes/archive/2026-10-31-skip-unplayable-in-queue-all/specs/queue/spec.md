## MODIFIED Requirements

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

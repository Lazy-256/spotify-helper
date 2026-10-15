## Purpose

Lets a caller read what is playing on the user's Spotify Connect device and control it: transport, volume, seek, shuffle, repeat, and starting playback of a given item.

## ADDED Requirements

### Requirement: State snapshot
`spotify-helper state` SHALL output one JSON object describing current playback:

- `active` (bool): whether Spotify reports an active playback device.
- `is_playing` (bool)
- `progress_ms` (int), `fetched_at_ms` (int, Unix time in milliseconds when the helper received the data)
- `shuffle` (bool), `repeat` (`"off"` | `"context"` | `"track"`)
- `device` (object or `null`): `id`, `name`, `type`, `volume_percent` (int or `null`)
- `context` (object or `null`): `type`, `uri`
- `item` (object or `null`): `type` (`"track"` | `"episode"`), `uri`, `id`, `name`, `artists` (array of `{name, uri}`), `album` (`{name, uri}`), `duration_ms`, `cover_url` (string or `null`)

For an episode, `artists` SHALL be empty and `album` SHALL describe the show.

#### Scenario: Track is playing
- **WHEN** a track is playing on a Connect device
- **THEN** `active` and `is_playing` are `true`, `device` and `item` are objects, and `item.type` is `"track"`

#### Scenario: Nothing active
- **WHEN** Spotify reports no active device
- **THEN** the command exits 0 with `active` `false`, `is_playing` `false`, `progress_ms` 0, and `device`, `context` and `item` all `null`

#### Scenario: Paused
- **WHEN** playback is paused
- **THEN** `active` is `true`, `is_playing` is `false`, and `item` is still populated

### Requirement: Cover image selection
`cover_url` SHALL be the URL of the smallest available image at least 300 px wide, or the largest available image when none reaches 300 px, or `null` when the item has no image. The same rule SHALL apply to every `cover_url` in all commands. Commands that return items can additionally provide a local `cover_path` when called with `--covers`, as defined by the `covers` capability.

#### Scenario: Several sizes
- **WHEN** Spotify lists images of 640, 300 and 64 px
- **THEN** `cover_url` is the 300 px image

#### Scenario: Only small images
- **WHEN** the only images are 64 and 32 px
- **THEN** `cover_url` is the 64 px image

### Requirement: Transport commands
`play`, `pause`, `next` and `prev` SHALL send the matching command to Spotify and output `{"ok":true}`. `toggle` SHALL pause when the current state is playing and resume otherwise. `play` SHALL resume the current playback and SHALL NOT change what is queued.

#### Scenario: Pause
- **WHEN** `spotify-helper pause` is run while playing
- **THEN** Spotify receives a pause request and the output is `{"ok":true}`

#### Scenario: Toggle while paused
- **WHEN** `spotify-helper toggle` is run while paused
- **THEN** playback resumes

#### Scenario: Nothing to control
- **WHEN** a transport command is run and Spotify reports no active device
- **THEN** the command exits 4 with `error.code` `no_active_device`

### Requirement: Volume
`spotify-helper volume <value>` SHALL accept an absolute percentage `0`–`100` or a signed relative change such as `+5` or `-10`. Relative values SHALL be applied to the current device volume and the result SHALL be clamped to `0`–`100`. Output SHALL be `{"ok":true,"volume_percent":<int>}` with the applied value. A value that is not an integer, or an absolute value outside `0`–`100`, SHALL fail with `usage`.

#### Scenario: Absolute volume
- **WHEN** the user runs `volume 40`
- **THEN** the device volume is set to 40 and the output has `volume_percent` 40

#### Scenario: Relative clamp
- **WHEN** the current volume is 97 and the user runs `volume +10`
- **THEN** the applied volume is 100

#### Scenario: Out of range
- **WHEN** the user runs `volume 150`
- **THEN** the command exits 2 with `error.code` `usage`

### Requirement: Seek
`spotify-helper seek <position>` SHALL accept an absolute position in milliseconds, or a signed relative offset such as `+10000` or `-5000` applied to the current progress. The resulting position SHALL be clamped to the range from 0 to one second before the end of the item (the duration minus 1000 ms, or 0 for an item shorter than that), so that a seek never ends the item; when the duration is unknown there SHALL be no upper limit. Output SHALL be `{"ok":true,"position_ms":<int>}` with the applied position.

#### Scenario: Skip back past the start
- **WHEN** progress is 3000 ms and the user runs `seek -5000`
- **THEN** the position is set to 0

#### Scenario: Absolute seek
- **WHEN** the user runs `seek 60000`
- **THEN** playback continues from 60000 ms

#### Scenario: Seek past the end
- **WHEN** an item lasts 200000 ms and the user runs `seek +50000` at 190000 ms, or `seek 999999`
- **THEN** the position is set to 199000 and the item keeps playing

#### Scenario: Very short item
- **WHEN** an item lasts 800 ms and the user runs `seek 5000`
- **THEN** the position is set to 0

### Requirement: Shuffle and repeat
`shuffle` SHALL accept `on`, `off` or `toggle` and `repeat` SHALL accept `off`, `context`, `track` or `cycle` (off → context → track → off). Output SHALL be `{"ok":true,"shuffle":<bool>}` and `{"ok":true,"repeat":"<mode>"}` respectively, holding the applied value. Any other argument SHALL fail with `usage`.

#### Scenario: Repeat cycle
- **WHEN** repeat is `"track"` and the user runs `repeat cycle`
- **THEN** repeat becomes `"off"`

#### Scenario: Bad argument
- **WHEN** the user runs `shuffle maybe`
- **THEN** the command exits 2 with `error.code` `usage`

### Requirement: Start playback of a URI
`spotify-helper play-uri <uri>` SHALL start playback of the given Spotify URI. A `spotify:track:` or `spotify:episode:` URI SHALL be played as a single item; a `spotify:album:`, `spotify:playlist:`, `spotify:artist:` or `spotify:show:` URI SHALL be played as a context from its beginning. `https://open.spotify.com/<type>/<id>` links SHALL be accepted and treated like the equivalent URI. Any other value SHALL fail with `usage`. Output SHALL be `{"ok":true}`.

#### Scenario: Play an album
- **WHEN** the user runs `play-uri spotify:album:abc`
- **THEN** Spotify is asked to play the album as a context and the output is `{"ok":true}`

#### Scenario: Play a track from a web link
- **WHEN** the user runs `play-uri https://open.spotify.com/track/abc`
- **THEN** the track `spotify:track:abc` is played

#### Scenario: Not a Spotify URI
- **WHEN** the user runs `play-uri hello`
- **THEN** the command exits 2 with `error.code` `usage`

### Requirement: Target device override
`play`, `pause`, `toggle`, `next`, `prev`, `volume`, `seek`, `shuffle`, `repeat` and `play-uri` SHALL accept `--device <id>`. When given, the request SHALL target that device instead of the active one, and the command SHALL NOT fail with `no_active_device` merely because no device is active.

#### Scenario: Explicit device
- **WHEN** no device is active and the user runs `play-uri spotify:track:abc --device D1`
- **THEN** the request targets device `D1`

### Requirement: Spotify error mapping for playback
A Spotify 404 with reason `NO_ACTIVE_DEVICE` SHALL map to `no_active_device`, a 403 with reason `PREMIUM_REQUIRED` SHALL map to `premium_required`, a 401 SHALL map to `not_authenticated`, and any other non-success status SHALL map to `api_error` with the HTTP status included in the message.

#### Scenario: Free account
- **WHEN** Spotify answers 403 `PREMIUM_REQUIRED`
- **THEN** the command exits 6 with `error.code` `premium_required`

## MODIFIED Requirements

### Requirement: Start playback of a URI
`spotify-helper play-uri <uri>` SHALL start playback of the given Spotify URI. A `spotify:track:` or `spotify:episode:` URI SHALL be played as a single item; a `spotify:album:`, `spotify:playlist:`, `spotify:artist:` or `spotify:show:` URI SHALL be played as a context from its beginning. `https://open.spotify.com/<type>/<id>` links SHALL be accepted and treated like the equivalent URI. Any other value SHALL fail with `usage`. Output SHALL be `{"ok":true}`.

`play-uri <track-or-episode> --context <uri>` SHALL instead start the context (`spotify:album:` or `spotify:playlist:`, or the equivalent link) at that item: the play request carries the context as `context_uri` and the item as `offset.uri`, so playback continues through the rest of the context. `--context` with a URI that is not a track or episode, a context that is not an album or playlist, or a value that is not a Spotify URI SHALL fail with `usage` before anything is sent. When the item is not part of the context Spotify's answer SHALL be reported with the shared error mapping. Output SHALL be `{"ok":true}`.

#### Scenario: Play an album
- **WHEN** the user runs `play-uri spotify:album:abc`
- **THEN** Spotify is asked to play the album as a context and the output is `{"ok":true}`

#### Scenario: Play a track from a web link
- **WHEN** the user runs `play-uri https://open.spotify.com/track/abc`
- **THEN** the track `spotify:track:abc` is played

#### Scenario: Not a Spotify URI
- **WHEN** the user runs `play-uri hello`
- **THEN** the command exits 2 with `error.code` `usage`

#### Scenario: Play from here in an album
- **WHEN** the user runs `play-uri spotify:track:t7 --context spotify:album:abc`
- **THEN** Spotify receives `context_uri` `spotify:album:abc` with `offset` `{"uri":"spotify:track:t7"}` and the output is `{"ok":true}`

#### Scenario: Play from here in a playlist
- **WHEN** the user runs `play-uri spotify:track:t7 --context https://open.spotify.com/playlist/pl1`
- **THEN** Spotify receives `context_uri` `spotify:playlist:pl1` with `offset` `{"uri":"spotify:track:t7"}`

#### Scenario: Context is not an album or playlist
- **WHEN** the user runs `play-uri spotify:track:t7 --context spotify:artist:a1`
- **THEN** the command exits 2 with `error.code` `usage` and nothing is sent to Spotify

#### Scenario: Context without a single item
- **WHEN** the user runs `play-uri spotify:album:abc --context spotify:playlist:pl1`
- **THEN** the command exits 2 with `error.code` `usage`

#### Scenario: Works with a device override
- **WHEN** the user runs `play-uri spotify:track:t7 --context spotify:album:abc --device D1`
- **THEN** the request targets device `D1`

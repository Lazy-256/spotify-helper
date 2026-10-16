# library Specification

## Purpose
Lets a caller browse the user's own playlists and saved albums, so the widget can offer them for playback without a search.

## Requirements

### Requirement: List playlists
`spotify-helper playlists` SHALL output `{"total":<int>,"offset":<int>,"limit":<int>,"playlists":[…]}` with the user's playlists in the order Spotify returns them. Each entry SHALL have `uri`, `id`, `name`, `owner_name` (string) and `cover_url` (string or `null`). Entries that Spotify returns as `null` SHALL be omitted.

#### Scenario: Playlists present
- **WHEN** the account has 3 playlists
- **THEN** `playlists` has 3 entries each with a playable `uri`, and `total` is 3

#### Scenario: No playlists
- **WHEN** the account has none
- **THEN** the output has `total` 0 and `"playlists":[]`

### Requirement: List saved albums
`spotify-helper albums` SHALL output `{"total":<int>,"offset":<int>,"limit":<int>,"albums":[…]}` with the user's saved albums in the order Spotify returns them. Each entry SHALL have the album fields of `search` (`uri`, `id`, `name`, `artists`, `release_date`, `total_tracks`, `cover_url`).

#### Scenario: Saved albums present
- **WHEN** the account has saved albums
- **THEN** every entry has the same fields as an album from `search`

### Requirement: Paging
Both commands SHALL accept `--limit <n>` (1–50, default 50) and `--offset <n>` (≥ 0, default 0). Values out of range or not integers SHALL fail with `usage`. The output's `limit` and `offset` SHALL echo the applied values and `total` SHALL be the full number of items available.

#### Scenario: Second page
- **WHEN** the user runs `playlists --limit 20 --offset 20`
- **THEN** the output has `limit` 20, `offset` 20 and entries starting at position 20

#### Scenario: Bad limit
- **WHEN** the user runs `albums --limit 0`
- **THEN** the command exits 2 with `error.code` `usage`

### Requirement: Library errors use the shared mapping
Both commands SHALL use the shared error mapping from `output-contract`, including `not_authenticated` and `rate_limited`.

#### Scenario: Not logged in
- **WHEN** the user runs `playlists` without a token
- **THEN** the command exits 3 with `error.code` `not_authenticated`

### Requirement: Save an item to the library
`spotify-helper save <uri>` SHALL add a single item to the user's library and output `{"ok":true,"uri":"<normalized-uri>"}`. Only `spotify:track:` and `spotify:episode:` URIs, or the equivalent `https://open.spotify.com/track/<id>` and `/episode/<id>` links, SHALL be accepted; any other value, including album, artist, playlist and show URIs, SHALL fail with `usage` before anything is sent to Spotify. A web link SHALL be normalized to its URI in the output. The helper SHALL send the request as `PUT /me/library` with the normalized URI in the `uris` query parameter. Saving an item that is already in the library SHALL succeed in the same way. The command SHALL NOT need an active device or a Premium account.

#### Scenario: Save a track
- **WHEN** the user runs `save spotify:track:abc` with a token that includes `user-library-modify`
- **THEN** Spotify receives `PUT /me/library` with `uris=spotify:track:abc` and the output is `{"ok":true,"uri":"spotify:track:abc"}`

#### Scenario: Save from a web link
- **WHEN** the user runs `save https://open.spotify.com/track/abc?si=xyz`
- **THEN** the output `uri` is `spotify:track:abc`

#### Scenario: Already saved
- **WHEN** the track is already in the library and the user runs `save` on it
- **THEN** the command exits 0 with `{"ok":true,"uri":"…"}`

#### Scenario: Nothing is playing
- **WHEN** Spotify reports no active device and the user runs `save spotify:track:abc`
- **THEN** the command still succeeds, because saving does not involve a device

#### Scenario: Album is rejected
- **WHEN** the user runs `save spotify:album:abc`
- **THEN** the command exits 2 with `error.code` `usage` and nothing is sent to Spotify

### Requirement: Remove an item from the library
`spotify-helper unsave <uri>` SHALL remove a single item from the user's library and output `{"ok":true,"uri":"<normalized-uri>"}`. It SHALL accept the same URIs and links as `save`, reject everything else with `usage`, and send `DELETE /me/library` with the normalized URI in the `uris` query parameter. Removing an item that is not in the library SHALL succeed in the same way.

#### Scenario: Remove a track
- **WHEN** the user runs `unsave spotify:track:abc` with a token that includes `user-library-modify`
- **THEN** Spotify receives `DELETE /me/library` with `uris=spotify:track:abc` and the output is `{"ok":true,"uri":"spotify:track:abc"}`

#### Scenario: Not in the library
- **WHEN** the track is not in the library and the user runs `unsave` on it
- **THEN** the command exits 0 with `{"ok":true,"uri":"…"}`

#### Scenario: Playlist is rejected
- **WHEN** the user runs `unsave spotify:playlist:abc`
- **THEN** the command exits 2 with `error.code` `usage` and nothing is sent to Spotify

### Requirement: Check whether an item is saved
`spotify-helper saved <uri>` SHALL output `{"uri":"<normalized-uri>","saved":<bool>}` where `saved` tells whether the item is in the user's library. It SHALL accept the same URIs and links as `save`, reject everything else with `usage`, and send `GET /me/library/contains` with the normalized URI in the `uris` query parameter, taking the first element of Spotify's boolean array. The command SHALL need only the read scope `user-library-read`.

#### Scenario: Track is saved
- **WHEN** Spotify answers `[true]` for `saved spotify:track:abc`
- **THEN** the output is `{"uri":"spotify:track:abc","saved":true}`

#### Scenario: Track is not saved
- **WHEN** Spotify answers `[false]`
- **THEN** the output is `{"uri":"spotify:track:abc","saved":false}`

#### Scenario: Token without the write scope
- **WHEN** the stored token has `user-library-read` but not `user-library-modify`
- **THEN** `saved` still works

#### Scenario: Unexpected answer
- **WHEN** Spotify answers with something that is not an array holding one boolean
- **THEN** the command exits 1 with `error.code` `api_error`

### Requirement: Saving and removing need the write scope
`save` and `unsave` SHALL check the scopes recorded in the stored token before contacting Spotify. When `user-library-modify` is not among them, the command SHALL fail with `error.code` `scopes_missing` and a message telling the user to run `spotify-helper login` again, and SHALL send no request to Spotify. The check SHALL run after the arguments have been validated, so a usage error is still reported as `usage`.

#### Scenario: Token from before the write scope existed
- **WHEN** the stored token lacks `user-library-modify` and the user runs `save spotify:track:abc`
- **THEN** the command exits 3 with `error.code` `scopes_missing` and no request is sent

#### Scenario: Bad URI wins over the scope check
- **WHEN** the stored token lacks `user-library-modify` and the user runs `save spotify:album:abc`
- **THEN** the command exits 2 with `error.code` `usage`

#### Scenario: Not logged in
- **WHEN** there is no stored token and the user runs `save spotify:track:abc`
- **THEN** the command exits 3 with `error.code` `not_authenticated`

### Requirement: Library write and check commands use the shared mapping
`save`, `unsave` and `saved` SHALL use the shared error mapping from `output-contract`, including `not_authenticated`, `rate_limited` and `network_error`. A refusal from Spotify that has no dedicated code, such as HTTP 403, SHALL surface as `api_error` with Spotify's message.

#### Scenario: Rate limited
- **WHEN** Spotify answers 429 with `Retry-After: 7` to `saved spotify:track:abc`
- **THEN** the command exits 5 with `error.code` `rate_limited` and `error.retry_after` 7

#### Scenario: Spotify refuses the request
- **WHEN** Spotify answers `save` with HTTP 403 although the token has the scope
- **THEN** the command exits 1 with `error.code` `api_error`

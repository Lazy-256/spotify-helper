## Purpose

Lets a caller look inside one catalog item: the tracks of an album or playlist, or the albums of an artist, in the entry shapes of `search`, so that a screen can show them and pass any `uri` back to `play-uri` or `queue-add`.

## ADDED Requirements

### Requirement: Album command
`spotify-helper album <uri>` SHALL output `{"album":<album>,"tracks":[<item>…],"total":<int>,"offset":<int>,"limit":<int>}`. `<album>` has the album fields of `search` (`uri`, `id`, `name`, `artists`, `release_date`, `total_tracks`, `cover_url`). Each `<item>` has the track fields of `search` (`type` `"track"`, `uri`, `id`, `name`, `artists`, `album` as `{name, uri}` of this album, `duration_ms`, `explicit`, `cover_url`) and the album's `cover_url`, and is in disc and track order. `total` SHALL be the number of tracks of the album and `offset` and `limit` SHALL echo the applied values. The argument SHALL be a `spotify:album:` URI or an `https://open.spotify.com/album/<id>` link (a trailing query string and an `intl-xx` path segment are tolerated); any other value SHALL fail with `usage`. Entries that Spotify returns as `null` SHALL be omitted.

#### Scenario: Album with tracks
- **WHEN** the user runs `album spotify:album:abc` and the album has 12 tracks
- **THEN** the output has `album.uri` `spotify:album:abc`, `total` 12, and `tracks` with 12 entries in track order, each with `album.uri` `spotify:album:abc`

#### Scenario: Web link
- **WHEN** the user runs `album https://open.spotify.com/album/abc?si=xyz`
- **THEN** the album `spotify:album:abc` is read

#### Scenario: Not an album
- **WHEN** the user runs `album spotify:playlist:abc` or `album hello`
- **THEN** the command exits 2 with `error.code` `usage` and nothing is sent to Spotify

#### Scenario: Track URIs are playable
- **WHEN** the user passes the `uri` of a returned track to `queue-add`
- **THEN** the command accepts it

### Requirement: Playlist command
`spotify-helper playlist <uri>` SHALL output `{"playlist":<playlist>,"tracks_available":<bool>,"tracks":[<item>…],"total":<int>,"offset":<int>,"limit":<int>}`. `<playlist>` has the playlist fields of `search` (`uri`, `id`, `name`, `owner_name`, `cover_url`) and `collaborative` (bool). `tracks_available` SHALL be `true` when Spotify returned the playlist's items and `false` when it returned only the playlist's metadata, which is what it does in Development Mode for playlists the user neither owns nor collaborates on, or when Spotify refuses the items request with 403 after the metadata was read; in the second case `tracks` SHALL be `[]` and `total` SHALL be 0, and the command SHALL succeed. Each `<item>` is a track or an episode with the fields of the `queue` items (`type`, `uri`, `id`, `name`, `artists`, `album`, `duration_ms`, `explicit`, `cover_url`) in playlist order; entries Spotify returns as `null`, and local files that have no URI, SHALL be omitted. The argument SHALL be a `spotify:playlist:` URI or an `https://open.spotify.com/playlist/<id>` link; any other value SHALL fail with `usage`.

#### Scenario: Own playlist
- **WHEN** the user runs `playlist spotify:playlist:abc` for a playlist they own with 30 items
- **THEN** `tracks_available` is `true`, `total` is 30, and `tracks` holds the first page of items

#### Scenario: Someone else's playlist
- **WHEN** Spotify returns the metadata of a playlist the user does not own and no items
- **THEN** the command exits 0 with `tracks_available` `false`, `"tracks":[]`, `total` 0, and the playlist's name, `owner_name` and `cover_url`

#### Scenario: Items refused
- **WHEN** Spotify returns the playlist's metadata and then answers 403 to the request for its items
- **THEN** the command exits 0 with `tracks_available` `false` and `"tracks":[]`

#### Scenario: Collaborative playlist
- **WHEN** the playlist is collaborative
- **THEN** `playlist.collaborative` is `true`, whether or not tracks were available

#### Scenario: Episodes and unavailable entries
- **WHEN** the playlist holds a podcast episode and an entry Spotify returns as `null`
- **THEN** the episode appears with `type` `"episode"` and the `null` entry does not

#### Scenario: Not a playlist
- **WHEN** the user runs `playlist spotify:album:abc`
- **THEN** the command exits 2 with `error.code` `usage`

### Requirement: Artist command
`spotify-helper artist <uri>` SHALL output `{"artist":<artist>,"albums":[<album>…],"total":<int>,"offset":<int>,"limit":<int>}`. `<artist>` has the artist fields of `search` (`uri`, `id`, `name`, `cover_url`). `albums` SHALL list the artist's albums and singles (not compilations and not appearances on other artists' releases) with the album fields of `search`, in the order Spotify returns them; `total` SHALL be the number of such releases. The output SHALL NOT contain top tracks, followers or popularity. The argument SHALL be a `spotify:artist:` URI or an `https://open.spotify.com/artist/<id>` link; any other value SHALL fail with `usage`.

#### Scenario: Artist with releases
- **WHEN** the user runs `artist spotify:artist:abc` and the artist has 25 albums and singles
- **THEN** the output has `artist.uri` `spotify:artist:abc`, `total` 25 and `albums` with the first page

#### Scenario: Artist without releases
- **WHEN** the artist has none
- **THEN** the output has `"albums":[]` and `total` 0

#### Scenario: Not an artist
- **WHEN** the user runs `artist spotify:track:abc`
- **THEN** the command exits 2 with `error.code` `usage`

### Requirement: Paging and covers
The three commands SHALL accept `--limit <n>` (1–50, default 50) and `--offset <n>` (≥ 0, default 0); out-of-range or non-integer values SHALL fail with `usage`. For `artist` Spotify serves at most 10 albums per request in Development Mode (a larger limit is answered with HTTP 400), so its default SHALL be 10 and a `--limit` from 11 to 50 SHALL be reduced to 10, as `search` reduces its limit; the output's `limit` is the applied value. `--limit` and `--offset` apply to the list the command returns (`tracks` or `albums`). With `--covers` every entry that has an image, and the header item (`album`, `playlist` or `artist`), SHALL carry `cover_path` as for `search --covers` (a local file or `null`); without it no `cover_path` appears. Tracks of an album SHALL share the album's cover, so the album's image is requested once.

#### Scenario: Second page
- **WHEN** the user runs `album spotify:album:abc --limit 10 --offset 10`
- **THEN** `limit` is 10, `offset` is 10 and `tracks` starts at the eleventh track

#### Scenario: Bad limit
- **WHEN** the user runs `artist spotify:artist:abc --limit 0`
- **THEN** the command exits 2 with `error.code` `usage`

#### Scenario: Artist page size
- **WHEN** the user runs `artist spotify:artist:abc` or `artist spotify:artist:abc --limit 20`
- **THEN** Spotify receives `limit=10` and the output's `limit` is 10

#### Scenario: Next page of an artist
- **WHEN** the artist has 33 releases and the user runs `artist spotify:artist:abc --offset 10`
- **THEN** the output has `total` 33, `offset` 10 and the next up to 10 albums

#### Scenario: Covers
- **WHEN** the user runs `album spotify:album:abc --covers`
- **THEN** `album.cover_path` and every track's `cover_path` are the same local file path (or `null`), and the cover is downloaded once

### Requirement: Catalog errors use the shared mapping
The three commands SHALL use the shared error mapping: `not_authenticated`, `rate_limited`, `network_error` and `api_error`. A Spotify 404 for an item that does not exist SHALL be an `api_error` whose message includes the status. The commands SHALL NOT need an active device, a Premium account or a scope beyond `login`'s, and SHALL NOT retry.

#### Scenario: Not logged in
- **WHEN** the user runs `album spotify:album:abc` without a token
- **THEN** the command exits 3 with `error.code` `not_authenticated`

#### Scenario: Rate limited
- **WHEN** Spotify answers 429 with `Retry-After: 12`
- **THEN** the command exits 5 with `error.code` `rate_limited` and `error.retry_after` 12

#### Scenario: Unknown album
- **WHEN** Spotify answers 404 for the album
- **THEN** the command exits 1 with `error.code` `api_error` and a message that includes 404

#### Scenario: A playlist Spotify does not serve at all
- **WHEN** the playlist is one of Spotify's own editorial playlists and Spotify answers 404 for it, which it does to apps in Development Mode
- **THEN** the command exits 1 with `error.code` `api_error` and a message that includes 404, the same as for a playlist that does not exist

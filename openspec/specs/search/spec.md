# search Specification

## Purpose
Lets a caller search the Spotify catalog for tracks, albums, artists and playlists and get compact results that can be shown in a list and passed back to `play-uri` or `queue-add`.

## Requirements

### Requirement: Search command
`spotify-helper search <query>` SHALL search Spotify and output one JSON object `{"query":"<q>", …}` containing one array per requested type: `tracks`, `albums`, `artists`, `playlists`. By default all four types SHALL be requested; `--type` with a comma-separated subset (`track,album,artist,playlist`) SHALL restrict them, and only requested types SHALL appear in the output, each as an array that is empty when nothing matched. Words of the query given as separate arguments SHALL be joined with single spaces. An empty query or an unknown type SHALL fail with `usage`.

#### Scenario: Default search
- **WHEN** the user runs `search daft punk`
- **THEN** the output has `query` `"daft punk"` and the arrays `tracks`, `albums`, `artists` and `playlists`

#### Scenario: Only artists
- **WHEN** the user runs `search daft punk --type artist`
- **THEN** the output contains `artists` and none of the other arrays

#### Scenario: Empty query
- **WHEN** the user runs `search` with no words
- **THEN** the command exits 2 with `error.code` `usage`

### Requirement: Result limit
Each array SHALL contain at most 10 entries. `--limit <n>` SHALL accept 1–10; larger values SHALL be reduced to 10 and a non-integer or non-positive value SHALL fail with `usage`. The default SHALL be 10.

#### Scenario: Limit above maximum
- **WHEN** the user runs `search x --limit 50`
- **THEN** each array has at most 10 entries

### Requirement: Result shapes
Entries SHALL have these fields, and every entry SHALL include `uri` suitable for `play-uri`:

- track: `uri`, `id`, `name`, `artists` (array of `{name, uri}`), `album` (`{name, uri}`), `duration_ms`, `explicit` (bool), `cover_url`
- album: `uri`, `id`, `name`, `artists` (array of `{name, uri}`), `release_date` (string), `total_tracks` (int), `cover_url`
- artist: `uri`, `id`, `name`, `cover_url`
- playlist: `uri`, `id`, `name`, `owner_name` (string), `cover_url`

`cover_url` follows the cover image selection rule of `playback-state`. Entries that Spotify returns as `null` SHALL be omitted.

#### Scenario: Null playlist entries
- **WHEN** Spotify returns `null` among the playlist results
- **THEN** the `playlists` array contains only the non-null entries

#### Scenario: Result is playable
- **WHEN** the user passes a `uri` from a search result to `play-uri`
- **THEN** the command accepts it

### Requirement: Search errors
Search SHALL use the shared error mapping: `not_authenticated`, `rate_limited`, `network_error` and `api_error`.

#### Scenario: Not logged in
- **WHEN** the user runs `search x` without a token
- **THEN** the command exits 3 with `error.code` `not_authenticated`

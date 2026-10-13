## Purpose

Lets a caller browse the user's own playlists and saved albums, so the widget can offer them for playback without a search.

## ADDED Requirements

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

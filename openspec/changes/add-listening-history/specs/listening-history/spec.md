## Purpose

Lets a caller show what the user listened to lately and what they listen to most, in the entry shapes of `search`, so that a screen can list it and pass any `uri` back to `play-uri` or `queue-add`.

## ADDED Requirements

### Requirement: Recently played command
`spotify-helper recent` SHALL output `{"limit":<int>,"cursors":{"before":<int|null>,"after":<int|null>},"items":[{"played_at":"<RFC 3339>","context":<ctx|null>,"track":<item>}…]}` with the user's recently played tracks, newest first, as Spotify returns them. `<item>` has the track fields of `search` (`type` `"track"`, `uri`, `id`, `name`, `artists`, `album`, `duration_ms`, `explicit`, `cover_url`); `<ctx>` is `{"type":"<album|playlist|artist|…>","uri":"<uri>"}` or `null` when the track was played outside a context. The same track MAY appear more than once. `--limit <n>` SHALL accept 1–50 (default 20); a non-integer or out-of-range value SHALL fail with `usage`. `--before <ms>` and `--after <ms>` SHALL take a Unix time in milliseconds, MAY NOT be combined (`usage`), and SHALL be passed to Spotify as given. `cursors.before` is the value to pass as `--before` for the next older page and is `null` when there is none; `cursors.after` likewise for newer items. Entries Spotify returns as `null` SHALL be omitted. Podcast episodes are not part of the history.

#### Scenario: History present
- **WHEN** the user runs `recent` and Spotify returns 3 plays
- **THEN** `items` has 3 entries newest first, each with `played_at`, `context` and a `track` with a playable `uri`

#### Scenario: Played outside a context
- **WHEN** a track was played from search
- **THEN** its `context` is `null`

#### Scenario: Older page
- **WHEN** the user runs `recent --before 1792000000000`
- **THEN** Spotify receives `before=1792000000000` and the output holds the plays before that time

#### Scenario: Both cursors
- **WHEN** the user runs `recent --before 1 --after 2`
- **THEN** the command exits 2 with `error.code` `usage` and nothing is sent to Spotify

#### Scenario: Bad limit
- **WHEN** the user runs `recent --limit 51` or `recent --limit 0`
- **THEN** the command exits 2 with `error.code` `usage`

#### Scenario: Nothing played yet
- **WHEN** Spotify returns no plays
- **THEN** the output has `"items":[]` and both cursors `null`, and the exit code is 0

### Requirement: Top items command
`spotify-helper top tracks` and `spotify-helper top artists` SHALL output `{"type":"<tracks|artists>","range":"<short|medium|long>","total":<int>,"offset":<int>,"limit":<int>,"tracks":[<item>…]}` or `{…,"artists":[<artist>…]}` with the user's most listened items for the range, in Spotify's order; only the array of the requested type SHALL appear. `<item>` is as in "Recently played command"; `<artist>` has `uri`, `id`, `name` and `cover_url`. `--range short|medium|long` (default `medium`) SHALL select about the last 4 weeks, 6 months or year (Spotify's `short_term`, `medium_term`, `long_term`); any other value SHALL fail with `usage`. `--limit <n>` (1–50, default 20) and `--offset <n>` (≥ 0, default 0) SHALL work as in the `library` commands and be echoed. A missing or unknown type argument SHALL fail with `usage`. The output SHALL NOT contain genres, followers or popularity. Entries Spotify returns as `null` SHALL be omitted.

#### Scenario: Top tracks
- **WHEN** the user runs `top tracks`
- **THEN** the output has `type` `"tracks"`, `range` `"medium"` and a `tracks` array, and no `artists` key

#### Scenario: Top artists for a year
- **WHEN** the user runs `top artists --range long`
- **THEN** Spotify receives `time_range=long_term`, the output has `range` `"long"` and an `artists` array with playable URIs

#### Scenario: Second page
- **WHEN** the user runs `top tracks --limit 10 --offset 10`
- **THEN** `limit` is 10, `offset` is 10 and the entries start at position 10

#### Scenario: Unknown range or type
- **WHEN** the user runs `top tracks --range week` or `top albums`
- **THEN** the command exits 2 with `error.code` `usage`

#### Scenario: Not enough listening yet
- **WHEN** Spotify has no top items for the account
- **THEN** the array is empty, `total` is 0 and the exit code is 0

### Requirement: The history commands need their scope
`recent` SHALL require the stored token to include `user-read-recently-played`, and `top` SHALL require `user-top-read`. When the scope is missing the command SHALL fail with `scopes_missing` (exit 3) without sending anything to Spotify, with a message telling the user to run `login` again. A missing or foreign token SHALL stay `not_authenticated`.

#### Scenario: Old token
- **WHEN** the stored token was issued before `user-top-read` was requested and the user runs `top tracks`
- **THEN** the command exits 3 with `error.code` `scopes_missing` and no request is sent

#### Scenario: Only one of the new scopes
- **WHEN** the token has `user-top-read` but not `user-read-recently-played`
- **THEN** `top tracks` works and `recent` fails with `scopes_missing`

### Requirement: Covers and errors
`--covers` on `recent` and `top` SHALL add `cover_path` to every entry that has an image, as for `search --covers` (a local file or `null`); without it no `cover_path` appears. Both commands SHALL use the shared error mapping (`not_authenticated`, `rate_limited` with `retry_after`, `network_error`, `api_error`) and SHALL NOT need an active device or Premium.

#### Scenario: Covers
- **WHEN** the user runs `top artists --covers`
- **THEN** each artist has `cover_path`

#### Scenario: Rate limited
- **WHEN** Spotify answers 429 with `Retry-After: 12`
- **THEN** the command exits 5 with `error.code` `rate_limited` and `error.retry_after` 12

#### Scenario: Not logged in
- **WHEN** the user runs `recent` without a token
- **THEN** the command exits 3 with `error.code` `not_authenticated`

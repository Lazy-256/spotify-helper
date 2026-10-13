## Purpose

Gives the user and the plugin an inexpensive way to see why the helper is not working, without contacting Spotify and without exposing secrets.

## ADDED Requirements

### Requirement: Doctor report
`spotify-helper doctor` SHALL output one JSON object and exit 0 whenever it was able to run, even when it found problems:

- `ok` (bool): `true` when `problems` is empty.
- `problems` (array of strings): drawn from `not_configured`, `not_authenticated`, `scopes_missing`, `rate_limited`.
- `config`: `client_id_set` (bool), `client_id_source` (`"env"` | `"file"` | `null`), `redirect_uri` (string).
- `token`: `present` (bool), `client_id_matches` (bool or `null` when no token), `has_refresh_token` (bool), `expires_in_s` (int, negative when expired, or `null` when no token), `scopes` (array of strings).
- `rate_limit`: `active` (bool), `retry_after` (int seconds remaining, 0 when inactive).
- `paths`: `config`, `token`, `cache` (absolute paths).

#### Scenario: Healthy
- **WHEN** a client ID is configured and a valid token with all required scopes exists
- **THEN** `ok` is `true` and `problems` is `[]`

#### Scenario: Nothing set up
- **WHEN** neither a client ID nor a token exists
- **THEN** the exit code is 0, `ok` is `false` and `problems` contains `not_configured`

#### Scenario: Token for another client
- **WHEN** the token was issued for a different client ID
- **THEN** `token.client_id_matches` is `false` and `problems` contains `not_authenticated`

#### Scenario: Scope missing
- **WHEN** the stored token lacks one of the required scopes
- **THEN** `problems` contains `scopes_missing`

#### Scenario: Cooling down
- **WHEN** a rate-limit cooldown is active
- **THEN** `rate_limit.active` is `true`, `rate_limit.retry_after` is positive and `problems` contains `rate_limited`

### Requirement: Doctor is offline and secret-free
`doctor` SHALL NOT send any network request and SHALL NOT refresh the token. Its output SHALL NOT contain the client ID, tokens or authorization codes; only whether the client ID is set and where it came from.

#### Scenario: No requests
- **WHEN** `doctor` runs with an expired token
- **THEN** no request reaches Spotify and the stored token is unchanged

#### Scenario: No secrets
- **WHEN** `doctor` runs with a configured client ID and token
- **THEN** neither value appears anywhere in stdout or stderr

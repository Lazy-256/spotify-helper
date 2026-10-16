# auth Specification

## Purpose
Lets the user authorize `spotify-helper` against their own Spotify Developer app with OAuth 2.0 PKCE and keeps the resulting tokens fresh, so that all other commands can run unattended.

## Requirements

### Requirement: Configuration provides the client ID
The helper SHALL read `client_id` from `$XDG_CONFIG_HOME/spotify-helper/config.toml` (default `~/.config/spotify-helper/config.toml`). The environment variable `SPOTIFY_HELPER_CLIENT_ID`, when non-empty, SHALL take precedence over the file. The redirect URI SHALL default to `http://127.0.0.1:8888/callback` and MAY be overridden by a `redirect_uri` key. Every command that needs Spotify, including `login`, SHALL fail with code `not_configured` when no client ID is available.

#### Scenario: No client ID
- **WHEN** neither the config file nor the environment provides a client ID and the user runs `spotify-helper state`
- **THEN** the command exits 3 with `error.code` `not_configured`

#### Scenario: Environment overrides file
- **WHEN** the config file holds one client ID and `SPOTIFY_HELPER_CLIENT_ID` holds another
- **THEN** the helper uses the environment value

### Requirement: Login runs the PKCE authorization flow
`spotify-helper login` SHALL perform the OAuth 2.0 Authorization Code flow with PKCE (`S256`) and no client secret. It SHALL request the scopes `user-read-playback-state user-modify-playback-state user-read-currently-playing playlist-read-private user-library-read user-library-modify`, include a random `state` value, listen for the redirect on the host and port of the redirect URI bound to `127.0.0.1` only, print the authorization URL to stderr, and try to open it in the user's browser. It SHALL accept only a callback whose `state` matches, exchange the code for tokens, store them, and output `{"ok":true,"scopes":["…"]}` with the granted scopes. It SHALL stop listening after the callback is handled or after 5 minutes.

#### Scenario: Successful login
- **WHEN** the user approves the request in the browser
- **THEN** the helper stores the tokens, outputs `{"ok":true,"scopes":[…]}` including `user-library-modify`, exits 0, and no longer listens on the port

#### Scenario: Authorization URL asks for the write scope
- **WHEN** `login` prints the authorization URL
- **THEN** its `scope` parameter contains `user-library-modify`

#### Scenario: User denies access
- **WHEN** the redirect carries `error=access_denied`
- **THEN** the command exits 3 with `error.code` `not_authenticated` and stores no tokens

#### Scenario: State mismatch
- **WHEN** a request reaches the callback with a `state` that does not match
- **THEN** the helper does not exchange the code and keeps waiting until a valid callback or the timeout

#### Scenario: Nobody completes the login
- **WHEN** 5 minutes pass without a valid callback
- **THEN** the command exits 3 with `error.code` `not_authenticated`

#### Scenario: Port already in use
- **WHEN** the redirect port cannot be bound
- **THEN** the command exits 1 with `error.code` `internal` and a message naming the port

#### Scenario: Token from an earlier release
- **WHEN** the stored token was issued before `user-library-modify` was requested
- **THEN** `doctor` lists `scopes_missing` in `problems` until the user runs `login` again

### Requirement: Tokens are stored privately
The helper SHALL store tokens in `$XDG_CONFIG_HOME/spotify-helper/token.json` containing at least the access token, refresh token, expiry time, granted scopes and the client ID they were issued for. The directory SHALL have mode `0700` and the file mode `0600`, and the file SHALL be replaced atomically so a crash never leaves a partial file.

#### Scenario: File permissions
- **WHEN** login completes
- **THEN** `token.json` has mode `0600` and its directory has mode `0700`

### Requirement: Tokens refresh transparently
Before sending a request, the helper SHALL refresh the access token when it expires within 60 seconds. When the refresh response carries no new refresh token the previous one SHALL be kept. Concurrent invocations SHALL NOT lose a rotated refresh token. When the refresh is rejected (for example the grant was revoked) the command SHALL fail with `not_authenticated`.

#### Scenario: Expired token
- **WHEN** the stored access token is expired and the refresh token is valid
- **THEN** the command refreshes, stores the new tokens and completes normally

#### Scenario: Revoked grant
- **WHEN** Spotify rejects the refresh token
- **THEN** the command exits 3 with `error.code` `not_authenticated`

#### Scenario: Two commands refresh at once
- **WHEN** two invocations find an expired token at the same time
- **THEN** both complete and the stored refresh token is valid afterwards

### Requirement: Missing or foreign tokens require login
Commands that call the Spotify API (every command except `login`, `logout`, `version`, `doctor`, `cover` and `--help`) SHALL fail with `not_authenticated` when no token file exists, when it is unreadable, or when it was issued for a different client ID than the configured one.

#### Scenario: Client ID changed
- **WHEN** the configured client ID differs from the one recorded in `token.json`
- **THEN** the command exits 3 with `error.code` `not_authenticated`

#### Scenario: Never logged in
- **WHEN** no `token.json` exists
- **THEN** the command exits 3 with `error.code` `not_authenticated`

### Requirement: Logout removes stored tokens
`spotify-helper logout` SHALL delete `token.json` and output `{"ok":true}`, also when no token file existed.

#### Scenario: Logout twice
- **WHEN** the user runs `logout` twice in a row
- **THEN** both runs exit 0 with `{"ok":true}`

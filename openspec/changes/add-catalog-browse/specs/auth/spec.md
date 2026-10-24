## MODIFIED Requirements

### Requirement: Login runs the PKCE authorization flow
`spotify-helper login` SHALL perform the OAuth 2.0 Authorization Code flow with PKCE (`S256`) and no client secret. It SHALL request the scopes `user-read-playback-state user-modify-playback-state user-read-currently-playing playlist-read-private playlist-read-collaborative user-library-read user-library-modify`, include a random `state` value, listen for the redirect on the host and port of the redirect URI bound to `127.0.0.1` only, print the authorization URL to stderr, and try to open it in the user's browser. It SHALL accept only a callback whose `state` matches, exchange the code for tokens, store them, and output `{"ok":true,"scopes":["…"]}` with the granted scopes. It SHALL stop listening after the callback is handled or after 5 minutes.

#### Scenario: Successful login
- **WHEN** the user approves the request in the browser
- **THEN** the helper stores the tokens, outputs `{"ok":true,"scopes":[…]}` including `user-library-modify` and `playlist-read-collaborative`, exits 0, and no longer listens on the port

#### Scenario: Authorization URL asks for the write scope
- **WHEN** `login` prints the authorization URL
- **THEN** its `scope` parameter contains `user-library-modify`

#### Scenario: Authorization URL asks for the collaborative-playlist scope
- **WHEN** `login` prints the authorization URL
- **THEN** its `scope` parameter contains `playlist-read-collaborative`

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

#### Scenario: A token without the collaborative-playlist scope keeps working
- **WHEN** the stored token was issued before `playlist-read-collaborative` was requested
- **THEN** every command that worked before still works, `doctor` does not list `scopes_missing` because of it, and `playlist` does not require it

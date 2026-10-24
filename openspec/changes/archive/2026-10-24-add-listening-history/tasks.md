## 1. Output types

- [x] 1.1 Add the result types for `recent` (`limit`, `cursors` with nullable `before` and `after`, items of `{played_at, context, track}`) and `top` (`type`, `range`, `total`, `offset`, `limit` and either `tracks` or `artists`) to `internal/output/model.go` with `Covers()` methods; verify with a test that the JSON field names match the spec, that only the array of the requested type appears, and that `cover_path` appears only after covers were requested

## 2. API client

- [x] 2.1 Implement `Client.Recent` (`GET /me/player/recently-played` with `limit`, `before` or `after`): map the plays, drop `null` entries, pass the cursors through as numbers or `null`; verify with `httptest` tests in `internal/api` for a normal page, a context-less play, an older page (`before` reaches Spotify), an empty history and a 429
- [x] 2.2 Implement `Client.Top` (`GET /me/top/{type}` with `time_range`, `limit`, `offset`) with the range table `short`/`medium`/`long` to `short_term`/`medium_term`/`long_term`; verify with tests for tracks, artists (no genres, followers or popularity in the output), a second page, an empty account and a 429

## 3. Commands

- [x] 3.1 Add `recent` to `internal/cli`: `--limit` (1–50, default 20), `--before`, `--after` (milliseconds, mutually exclusive), `--covers`, `RequireScope("user-read-recently-played")` before any request; verify with CLI tests through `cli.Env` and `httptest` for every scenario of "Recently played command" and "The history commands need their scope"
- [x] 3.2 Add `top tracks` and `top artists`: `--range`, `--limit` (1–50, default 20), `--offset`, `--covers`, `RequireScope("user-top-read")` first, `usage` for an unknown type or range; verify with CLI tests for every scenario of "Top items command", including a token that has only one of the two scopes
- [x] 3.3 Verify the shared behaviors: `not_authenticated` without a token, `rate_limited` with `retry_after`, `network_error`, and that no request is sent when the scope is missing or an argument is wrong

## 4. Live check

- [x] 4.1 Run `recent`, `recent --before <cursor>`, `top tracks`, `top artists --range long` once on the real account after `login` with the new scopes, and keep the answers as fixtures under `internal/api/testdata` (ids and names replaced); note what differs from the spec (cursor values, `context` for a play from search, an empty top) and update this change's artifacts before continuing (done 2026-10-24, see the findings in `design.md`; nothing differed from the spec)

## 5. Login scopes

- [x] 5.1 Add `user-read-recently-played` and `user-top-read` to `auth.Scopes` with constants for `RequireScope`; verify with `internal/auth` tests that the authorization URL carries both and that `login` lists them, and with a `doctor` test that a token without them produces no `scopes_missing`

## 6. Documentation and checks

- [x] 6.1 Update `README.md` (commands table, output shapes, the two new scopes and the need to run `login` again, the empty-list and private-session notes) and `AGENTS.md` (scopes, structure, findings of the live check); verify by reading both for stale statements about the scope list
- [x] 6.2 Run `go build ./...`, `go vet ./...`, `gofmt -l .` and `go test -race ./...` and verify they pass with no output from `gofmt`

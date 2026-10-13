## 1. Foundations

- [ ] 1.1 Create `cmd/spotify-helper/main.go` with subcommand dispatch, `--help` text and `version`; verify `go build ./...` succeeds and `spotify-helper version` prints `{"version":"…"}`
- [ ] 1.2 Add `internal/output`: `Error` type (code, message, retry_after), exit-code table, JSON writers for stdout and stderr; verify unit tests cover every code→exit mapping and the "nothing on stdout on failure" rule
- [ ] 1.3 Add unknown-subcommand and missing-argument handling through `usage`; verify tests for `frobnicate` and `volume` without a value (exit 2)
- [ ] 1.4 Add the Spotify URI/link parser with per-command allowed types; verify table tests for `spotify:` URIs, `open.spotify.com` links with `?si=` and `intl-xx/`, and rejects
- [ ] 1.5 Add the config loader (flat TOML subset, `SPOTIFY_HELPER_CLIENT_ID` override, default redirect URI, XDG paths); verify tests for missing id, file value, env override, unsupported syntax

## 2. Authentication: login flow

- [ ] 2.1 Implement PKCE verifier/challenge generation and authorize-URL builder with the required scopes; verify tests against the RFC 7636 example vector and the exact scope string
- [ ] 2.2 Implement the loopback callback server bound to `127.0.0.1` with `state` check, 5-minute timeout and port-in-use error; verify tests for success, denied, state mismatch, timeout and busy port
- [ ] 2.3 Implement the code→token exchange against an injectable token endpoint and the `login` command output `{"ok":true,"scopes":[…]}`; verify an `httptest` end-to-end test of `login` with a simulated browser callback
- [ ] 2.4 Implement best-effort `xdg-open` and printing the URL to stderr only; verify stdout stays a single JSON document in the login test

## 3. Authentication: tokens and refresh

- [ ] 3.1 Implement the token store (`0700` dir, `0600` file, atomic replace, recorded `client_id`); verify tests for permissions, crash-safe replace and client-ID mismatch → `not_authenticated`
- [ ] 3.2 Implement proactive refresh (60 s margin, keep old refresh token when none returned, `invalid_grant` → `not_authenticated`); verify tests with a fake clock and fake token endpoint
- [ ] 3.3 Add `flock` around refresh with re-read after locking; verify a test that runs concurrent refreshes against a rotating fake endpoint and ends with a valid refresh token
- [ ] 3.4 Implement `logout` (idempotent); verify running it twice exits 0 both times

## 4. API client and rate limits

- [ ] 4.1 Implement `api.Client` (bearer auth, 10 s timeout, injectable base URLs, JSON decode, 204 handling); verify tests with `httptest` including timeout → `network_error`
- [ ] 4.2 Implement Spotify status/reason → error-code mapping (`NO_ACTIVE_DEVICE`, `PREMIUM_REQUIRED`, 401, other); verify one table test per mapping
- [ ] 4.3 Implement the on-disk 429 cooldown (default 30 s, min 1 s, keep later deadline, local fail-fast with remaining seconds, no retry); verify tests for header present, missing, malformed, active cooldown and expired cooldown, and that no request is sent while cooling down
- [ ] 4.4 Implement the shared `Item`/`Artist`/cover-selection types and wire→output mapping helpers; verify tests for 640/300/64, small-only and no-image cases and for episodes (empty `artists`, show as `album`)

## 5. Playback state and controls

- [ ] 5.1 Implement `state` (active, nothing-active → nulls, paused, fetched_at_ms); verify tests for track, episode, paused and 204
- [ ] 5.2 Implement `play`, `pause`, `next`, `prev`, `toggle` with `--device`; verify tests for requests sent, toggle both directions, `no_active_device`, and that `--device` bypasses it
- [ ] 5.3 Implement `volume` (absolute, relative, clamp, validation) and `seek` (absolute, relative, clamp to duration); verify tests for each scenario in the spec
- [ ] 5.4 Implement `shuffle on|off|toggle` and `repeat off|context|track|cycle`; verify tests including cycle order and bad arguments
- [ ] 5.5 Implement `play-uri` (single item vs context, web links, usage errors); verify tests asserting the request body for track, album and playlist

## 6. Search

- [ ] 6.1 Implement `search` with `--type` and `--limit` (clamp to 10, validation, word joining); verify tests for default types, subset, empty query and limit 50
- [ ] 6.2 Implement result mapping for tracks, albums, artists and playlists, dropping `null` entries; verify tests with a fixture containing nulls and missing images

## 7. Queue and devices

- [ ] 7.1 Implement `queue` (now_playing + queue, empty case); verify tests for items present and nothing playing
- [ ] 7.2 Implement `queue-add` (track/episode only, link normalization, `--device`); verify tests for track, link with `?si=`, album rejected without any request, and `no_active_device`
- [ ] 7.3 Implement `devices`; verify tests for two devices, empty list and a restricted device with `null` id
- [ ] 7.4 Implement `transfer` (id then case-insensitive name, ambiguity, restricted excluded, `--play`, `premium_required`); verify tests for each scenario in the spec

## 8. Covers

- [ ] 8.1 Implement the cover cache (URL→path hashing, https-only, no credentials, `image/*` and 2 MiB limits, temp+rename, `Chtimes` on hit) and the `cover <url>` command; verify tests for first download, cache hit (no request), http URL → `usage`, `text/html` and oversized bodies → `api_error` with nothing cached, and no `Authorization` header
- [ ] 8.2 Implement bounded cleanup (30 days, 100 MiB LRU, once per day, tolerant of concurrent removal); verify tests with a fake clock and files removed mid-cleanup
- [ ] 8.3 Implement the `--covers` post-processing step (dedupe, 6 workers, one 8 s deadline, `cover_path` null on failure, field absent without the flag) and wire it into `state`, `queue`, `search`, `playlists` and `albums`; verify tests for warm cache, one failing image, and flag omitted

## 9. Library

- [ ] 9.1 Implement `playlists` and `albums` with `--limit` (1–50) and `--offset`, `total/offset/limit` echo and `null` entry skipping; verify tests for populated, empty, second page and `--limit 0`

## 10. Diagnostics

- [ ] 10.1 Implement `doctor` (config, token, scopes, cooldown, paths; offline; always exit 0 when it ran); verify tests for healthy, nothing set up, foreign-client token, missing scope, active cooldown, and that stdout/stderr contain neither the client ID nor token values and no request is made

## 11. Real-account smoke test and docs

- [ ] 11.1 With the user's own Client ID, run `login` and every command against a real Connect device and record which endpoints and fields behave differently in Development Mode; verify with a written checklist in `README.md` and fix or document each deviation
- [ ] 11.2 Verify no secret appears in any output: run the full command set with a real token and `rg` stdout/stderr captures for token and client-ID values
- [ ] 11.3 Document the commands, JSON shapes, error codes and recommended polling interval in `README.md`, and update `AGENTS.md` structure/decisions; verify every command in the specs appears in the README
- [ ] 11.4 Run `go vet ./...`, `gofmt -l .` and `go test ./...`; verify all pass with no output from `gofmt -l`

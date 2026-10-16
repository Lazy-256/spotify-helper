## 1. Manual check of the endpoints

- [x] 1.1 With the real account and a throw-away token that has `user-library-modify`, call `PUT`, `GET …/contains` and `DELETE` on `/me/library?uris=spotify:track:<id>` (for example with curl) and record the status codes and the contains body in design.md; if Development Mode blocks them, stop and revisit decision 1 — Checked through the built helper instead of curl: Development Mode accepts all three endpoints (`save` and `unsave` exit 0, `saved` follows every write at once); raw status codes were not recorded.

## 2. Scope and error code

- [x] 2.1 Add `user-library-modify` to `auth.Scopes` and verify that `go test ./internal/auth` passes with the authorization-URL scenario (the `scope` parameter contains the new scope)
- [x] 2.2 Add error code `scopes_missing` with exit code 3 in `internal/output` and verify that the output tests cover the new row of the exit-code table
- [x] 2.3 Verify that `doctor` reports `scopes_missing` for a stored token that lacks the new scope (extend `internal/cli/doctor_test.go`)

## 3. API calls

- [x] 3.1 Add `SaveItem`, `RemoveItem` and `ItemSaved` to `internal/api/library.go` using `PUT`/`DELETE /me/library` and `GET /me/library/contains` with the normalized URI in `uris`; verify with `internal/api` tests against a fake server (method, path, query, empty 200 body, one-boolean array, malformed array becomes `api_error`)

## 4. Commands

- [x] 4.1 Add `save` and `unsave` to `internal/cli/library.go`: parse with `ParseAllowed(Track, Episode)`, then check the stored token's scopes, then call the API, print `{"ok":true,"uri":…}`; verify with CLI tests for track, web link, already saved, album rejected (`usage`, no request), missing scope (`scopes_missing`, no request), no token (`not_authenticated`), bad URI winning over the scope check
- [x] 4.2 Add `saved` with output `{"uri":…,"saved":bool}`; verify with CLI tests for true, false, a token that has only the read scope, and `rate_limited` with `retry_after`
- [x] 4.3 Verify that `spotify-helper --help` and the usage text list the three commands, and that `go vet ./...` and `go test ./...` pass

## 5. Docs and live check

- [x] 5.1 Update `README.md` (command table, scope list, error code `scopes_missing`, note that the user must run `login` again, note on `contains` right after a write) and `AGENTS.md` (scope list) in the same commit; verify with `rg user-library` that no stale scope list remains
- [x] 5.2 Build, run `spotify-helper doctor` and confirm `scopes_missing`, run `login`, then confirm `saved` → `save` → `saved` → `unsave` → `saved` against the real account returns `false`, `ok`, `true`, `ok`, `false` — Done with the real account: `doctor` reported `scopes_missing` before `login` and `ok` after; the write chain worked, but the track was liked by the user in the Spotify app just before the run, so the observed answers were `true, ok, true, ok, false` rather than `false, ok, true, ok, false`; the like was restored afterwards.

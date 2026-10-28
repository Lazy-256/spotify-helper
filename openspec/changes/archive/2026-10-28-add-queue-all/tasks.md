## 1. Output types

- [x] 1.1 Add the result type of `queue-add-all` (`ok`, `uri`, `added`, `skipped`, `total`, `offset`, `truncated`) and the optional `Added` and `Total` fields of `output.Error` (omitted when zero or unset, written next to `retry_after`), and the code `tracks_unavailable`; verify with `internal/output` tests that the field names match the spec, that the error object has neither field unless set, and that the new code exits with 1

## 2. API client

- [x] 2.1 Implement `Client.QueueAddAll(ctx, u, offset, device)`: read pages of 50 from `offset` (`Album` or `Playlist`), queue each item with `QueueAdd` strictly in order, stop at 100 added, count skipped entries, compute `truncated`, return `tracks_unavailable` when a playlist has `TracksAvailable=false`, and on a failure after at least one item attach `Added` (and `Total` when known) to the `*output.Error`; verify with `httptest` tests in `internal/api` for an album, an offset, a playlist over the cap (exactly 100 posts, the third page never read), skipped local files, an offset past the end, an unreadable playlist, and a 429 on the ninth post (eight accepted, no tenth request, error carries `added` 8)
- [x] 2.2 Check ordering: verify with a test that the posted URIs arrive in container order and never overlap in time

## 3. Command

- [x] 3.1 Add `queue-add-all <album|playlist uri|link> [--offset N] [--device ID]` to `internal/cli/queue.go`: accept only album and playlist, reject other types and a bad offset with `usage` before any request, normalize web links; verify with CLI tests through `cli.Env` and `httptest` for every scenario of "Add a whole album or playlist", "Playlists whose items are not served" and "A failure part-way reports what was queued"
- [x] 3.2 Verify the shared behaviors: `not_authenticated` without a token, `rate_limited` from an active cooldown without any request and without `added`, `no_active_device` from the first post, `network_error`

## 4. Live check

- [x] 4.1 On the real account: run `queue-add-all` for a short album, for the same album with `--offset`, and for a playlist of 100+ items; record the time, the order in `queue`, whether a 429 appears at 100 posts, and what `skipped` and `total` report for a playlist with a local file if the account has one; update this change's artifacts before continuing and keep a trimmed fixture if the answers differ from the tests' assumptions (done 2026-10-28, see "Live check findings" in `design.md`; no local-file playlist was available, so that case stays covered by tests only)

## 5. Documentation and checks

- [x] 5.1 Update `README.md` (commands table, output shape, the error codes with `added`/`total`, the cap, the order, the note that `queue-add` puts items after the existing user queue) and `AGENTS.md` (structure, findings of the live check); verify by reading both for stale statements about the queue
- [x] 5.2 Run `go build ./...`, `go vet ./...`, `gofmt -l .` and `go test -race ./...` and verify they pass with no output from `gofmt`

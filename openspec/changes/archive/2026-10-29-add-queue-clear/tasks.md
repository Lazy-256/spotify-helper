## 1. Output types

- [x] 1.1 Add the result type of `queue-clear` (`ok`, `skipped`, `empty`) and the optional `Skipped` field of `output.Error` (omitted when 0, written next to `added`/`total`); verify with `internal/output` tests that the field names match the spec and that the error object has no `skipped` unless set

## 2. API client

- [x] 2.1 Implement `Client.QueueClear(ctx, device)`: read the state (`no_active_device` when inactive or without a current item), start the current item again as its own context, loop (read the queue, stop when empty, send one `next` per entry shown, at most 100 in total), read the queue once more to set `empty`, pause at the end; on a failure after the context was dropped attach `Skipped` to the `*output.Error` and skip the final pause for `rate_limited` and `network_error`; verify with `httptest` tests in `internal/api` for a queue of three, an empty queue, 250 entries (exactly 100 `next` in batches of at most 20 with reads between), nothing playing (no request after the state read), a 429 on the 31st `next` (`skipped` 30, nothing after it), the pause being the last request, the request order never overlapping, the queue padded with copies of the playing item (three real entries, exactly three `next`, `empty` true), a device that applies `next` late (no `next` past the end), a refused `next` at the end (success), one that passes on retry, and one that does not (error after four attempts), and a refused final pause (success)
- [x] 2.2 Verify with a test that the device id is passed to every request that takes one

## 3. Command

- [x] 3.1 Add `queue-clear [--device ID]` to `internal/cli/queue.go` with no positional arguments (anything else is `usage` before any request); verify with CLI tests through `cli.Env` and `httptest` for every scenario of "Clear the queue" and "A failure part-way reports what was skipped", including a cooldown from an earlier 429, `not_authenticated`, a padded queue, and that `queue-clear` and `queue-add-all` run under a budget of about 120 s while the other commands keep 30 s

## 4. Live check

- [x] 4.1 On the real account with a long test queue: run `queue-clear` once and record how long it took, whether it emptied the queue in one run or stopped at the cap, whether `queue` is empty afterwards or shows suggestions (autoplay), and what the device does (position of the current item, paused at the end); run it again after a partial run; update this change's artifacts before continuing (done 2026-10-29: three defects found and fixed, see "Live check findings" in `design.md`)

## 5. Documentation and checks

- [x] 5.1 Update `README.md` (commands table, output shape, `error.skipped`, the side effects: restart, context dropped, blips, pause at the end, the cap) and `AGENTS.md` (structure, findings of the live check and the spike); verify by reading both for the statement "the queue cannot be emptied"
- [x] 5.2 Run `go build ./...`, `go vet ./...`, `gofmt -l .` and `go test -race ./...` and verify they pass with no output from `gofmt`

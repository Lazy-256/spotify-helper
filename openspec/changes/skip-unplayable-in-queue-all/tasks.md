## 1. API client

- [x] 1.1 Read `is_playable` into `wireItem` as `*bool`; split `Album` and `Playlist` into unexported variants that take a `market` and, when it is set, drop entries whose `IsPlayable` is a non-nil `false` (the public methods call them with an empty market and behave exactly as before); verify with `internal/api` tests that `album`/`playlist` requests carry no `market` and keep returning every track, and that the variants send `market=from_token` and drop only the entries marked `false`
- [x] 1.2 Make `containerPage` use the market variants with `from_token`; verify with `httptest` tests in `internal/api` for an album with one unplayable track (`added` 6, `skipped` 1, `total` 7, six posts in order), a playlist with unplayable and local entries mixed (`skipped` counts both), unplayable entries not using up the cap (130 items, 5 unplayable among the first 100: exactly 100 posts, `truncated` true), an entry without the mark being queued, and a page where every entry is unplayable (`added` 0, the run continues to the next page)

## 2. Command

- [x] 2.1 Verify through `cli.Env` and `httptest` that `queue-add-all` sends `market=from_token` on the tracks/items request, that the output reports the unplayable track in `skipped`, and that `album`, `playlist` and `queue-add` requests are unchanged

## 3. Live check

- [x] 3.1 On the real account (`la-box`, explicit filter on): run `queue-add-all` for the 7-track album `spotify:album:2nUfOkrhJH3wlqpfR6qfmE` and check that the queue holds 6 tracks without "Ласточка" and the output says `added` 6, `skipped` 1; run it for a playlist that contains an unplayable track and check that the mark is present on playlist items; record whether any track comes back relinked (a `uri` different from the one `album` shows); update this change's artifacts before continuing if playlists do not carry the mark

## 4. Documentation and checks

- [x] 4.1 Update `README.md` (`queue-add-all`: `skipped` includes tracks not playable for the user, with the explicit-filter example) and `AGENTS.md` (findings of the live check); verify by reading both for statements that `skipped` is only local files
- [x] 4.2 Run `go build ./...`, `go vet ./...`, `gofmt -l .` and `go test -race ./...` and verify they pass with no output from `gofmt`

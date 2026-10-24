## 1. Output types and URI handling

- [x] 1.1 Add the result types for `album`, `playlist` and `artist` to `internal/output/model.go` (header entry, list, `total`, `offset`, `limit`, `tracks_available` and `collaborative` for playlists) with `Covers()` methods like `SearchResult`; verify with a test in `internal/output` that the JSON field names and order match the spec and that `cover_path` appears only after covers were requested
- [x] 1.2 Check in `internal/spotifyuri` that `album`, `playlist` and `artist` URIs and `open.spotify.com` links (with `?si=` and an `intl-xx` segment) parse to the right type and that other types are refused; add the missing cases to `spotifyuri_test.go` and verify `go test ./internal/spotifyuri` passes

## 2. API client

- [x] 2.1 Implement `Client.Album` (`GET /albums/{id}` then `GET /albums/{id}/tracks` with `limit` and `offset`): tracks get the album as `album` and its cover, `null` entries are dropped; verify with `httptest` tests in `internal/api` for a normal page, a second page, an album with no tracks and a 404
- [x] 2.2 Implement `Client.Artist` (`GET /artists/{id}` then `GET /artists/{id}/albums` with `include_groups=album,single`, `limit`, `offset`); verify with tests for a normal page, no releases, `null` entries and a 404
- [x] 2.3 Implement `Client.Playlist` (`GET /playlists/{id}`, then `GET /playlists/{id}/items` only when the response carries items): `item` with `track` fallback, skipping `null`, local and non-track/episode entries, `tracks_available` false for a response without items and for a 403 on the items request, `collaborative` from the playlist object; verify with tests for an own playlist, a response without items (the second request must not be sent), a 403 on items, an episode, a local file, the old `track` key and a 404
- [x] 2.4 Extend the play request with the context offset (`context_uri` plus `offset.uri`) for an item inside an album or playlist; verify with a test that the request body has exactly `context_uri` and `offset` and that `--device` still goes in the query

## 3. Commands

- [x] 3.1 Add `album`, `playlist` and `artist` to `internal/cli` with `--limit` (1–50, default 50), `--offset`, `--covers`, URI validation (`usage` for the wrong type, before any request), and the shared error mapping; verify with CLI tests through `cli.Env` and `httptest` for each scenario of the `catalog` spec (output fields, second page, bad limit, wrong type, not authenticated, 429 with `retry_after`, 404)
- [x] 3.2 Wire `--covers` for the three commands through `internal/covers` (album tracks share one download); verify with a test that an album with 12 tracks downloads its cover once and every `cover_path` is the same file, and that no `cover_path` appears without the flag
- [x] 3.3 Add `--context <uri>` to `play-uri`: only for a track or episode, only an album or playlist context, links accepted, `usage` before any request otherwise; verify with CLI tests for the scenarios of the `playback-state` delta (album, playlist link, artist context refused, album as the item refused, with `--device`)

## 4. Live check

- [ ] 4.1 Run each new request once with the real account and record the answers as fixtures under `internal/api/testdata` (ids and names replaced): `album` and `artist` of any release, `playlist` of an own playlist, a followed one and a Spotify-made one, and `play-uri <track> --context <album>` on the active device; note what differs from the spec (availability of `/albums/{id}/tracks` and `/artists/{id}/albums`, the shape of a playlist without items, `total` when items are absent, a 403) and update the artifacts of this change before continuing

## 5. Login scope

- [x] 5.1 Add `playlist-read-collaborative` to `auth.Scopes`; verify with `internal/auth` tests that the authorization URL carries it, and with a `doctor` test that a token without it does not produce `scopes_missing`

## 6. Documentation and checks

- [ ] 6.1 Update `README.md` (commands table, output shapes, the Development Mode limits for playlists and artists, `login` once more for collaborative playlists, `play-uri --context`) and `AGENTS.md` (scopes, structure, the facts found in the live check); verify by reading both for stale statements such as "no way to list the contents of an album"
- [x] 6.2 Run `go build ./...`, `go vet ./...`, `gofmt -l .` and `go test -race ./...` and verify they pass with no output from `gofmt`

## Why

A Noctalia v5 widget needs to control Spotify playing on a Connect device:
show state, transport controls, search, queue, and device switching. MPRIS
cannot see or move Connect devices, and the `spotify_player` CLI has no
add-to-queue command and takes 24–60 s per call without a running client.
A small purpose-built Go CLI over the Spotify Web API removes both limits and
gives the Luau plugin one fast, stable, JSON-speaking dependency.

## What Changes

- New Go CLI `spotify-helper` (standard library only) with subcommands:
  `login`, `logout`, `state`, `play`, `pause`, `toggle`, `next`, `prev`,
  `volume`, `seek`, `shuffle`, `repeat`, `play-uri`, `search`, `queue`,
  `queue-add`, `devices`, `transfer`, `playlists`, `albums`, `cover`,
  `doctor`, `version`.
- Opt-in local cover cache: `--covers` on item-returning commands adds a
  `cover_path` to a downloaded file, and `cover <url>` caches one image,
  because Noctalia's `ui.image` loads local files only.
- Library browsing (`playlists`, `albums`) and an offline `doctor` report.
- Own OAuth 2.0 Authorization Code + PKCE flow with the user's own
  `client_id` (no client secret), loopback redirect
  `http://127.0.0.1:8888/callback`, token stored with mode `0600` and refreshed
  automatically.
- A single JSON output and error contract shared by every command, including
  HTTP 429 `Retry-After` handling with no automatic retries.
- Project scaffolding: Go module, README, AGENTS.md, OpenSpec config.

## Capabilities

### New Capabilities
- `output-contract`: stdout JSON, structured JSON errors on stderr, exit codes, rate-limit and no-retry behaviour shared by all commands.
- `auth`: `login` PKCE flow, config and token files, silent refresh, auth-related error codes.
- `playback-state`: `state` snapshot, transport/volume/seek/shuffle/repeat controls and starting playback of a URI, on the active or a chosen Connect device.
- `search`: catalog search for tracks, albums, artists and playlists, limited to 10 results per type.
- `queue`: read the playback queue and add a track or episode to it.
- `devices`: list Connect devices and transfer playback to one of them by id or name.
- `covers`: cache album/playlist/artist images on disk and expose them as local `cover_path`, via `cover <url>` and the `--covers` flag.
- `library`: list the user's playlists and saved albums with paging, ready for `play-uri`.
- `diagnostics`: `doctor`, an offline self-check of configuration, token and rate-limit state without printing secrets.

### Modified Capabilities

None. This is the first change; `openspec/specs/` is empty.

## Non-goals

- MPRIS or any local-player integration.
- A long-running daemon, caching of playback state, or push updates; callers poll `state`.
- Playlist or library editing, recommendations, lyrics.
- Any UI; the Luau plugin lives in a separate repository.
- Reading or sharing `spotify_player`'s tokens or credentials.
- Liked-songs commands (`like`, `unlike`, a `liked` field): deferred until the
  library endpoints are confirmed against a real Development Mode account.
- Listing the tracks of an album or playlist.

## Impact

- New code under `cmd/spotify-helper/` and `internal/{auth,api,output}/`.
- New files on the user's machine: `$XDG_CONFIG_HOME/spotify-helper/{config.toml,token.json}`, and `$XDG_CACHE_HOME/spotify-helper/{rate-limit.json,covers/}`.
- Network: `accounts.spotify.com` and `api.spotify.com` over HTTPS, plus Spotify's image CDN for covers (without credentials); local listener on `127.0.0.1:8888` only during `login`.
- Requires Spotify Premium and a Spotify Developer Dashboard app (Development Mode limits apply).
- **Output contract**: introduced here for the first time; later changes to it are breaking for the plugin.

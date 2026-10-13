## Context

`spotify-helper` is a short-lived CLI: every call is a new process started by
the Noctalia plugin (or a human), talks to the Spotify Web API over HTTPS and
exits. See `proposal.md` for the motivation. Constraints that shape the design:

- Playback runs on a Connect device, so everything goes through the Web API.
- Spotify Development Mode: Premium owner, search capped at 10, some catalog
  endpoints removed, 429 with `Retry-After` (see `AGENTS.md`).
- Many processes can run at once (the plugin polls `state` while the user
  clicks buttons), so shared on-disk state (token, cooldown) must be safe under
  concurrency.
- Noctalia's `ui.image` loads **local files only**; the plugin API offers
  `noctalia.download(url, dest, cb)`, so the plugin could fetch covers itself.
  The helper still provides a cache because search/library lists can show up to
  40 covers at once and need a shared, deduplicated, size-bounded cache.
- Linux only (the consumer is a Wayland shell); POSIX file locking is fine.
- Standard library only; Go version per `go.mod`.

## Goals / Non-Goals

**Goals:**
- Cheap to start and fast per call: one HTTPS request for most commands.
- One central place that turns any failure into the error contract of the
  `output-contract` spec.
- Spotify's wire format isolated behind our own output types so an API change
  does not silently change the plugin contract.
- Fully testable without network or real credentials.

**Non-Goals:**
- Daemon, caching of playback state, push updates (callers poll).
- Queue removal or reordering (the API has none), playlist/library editing.
- Windows/macOS support.

## Decisions

### Layout and CLI parsing
`cmd/spotify-helper/main.go` dispatches the first argument to a command
function; each command owns a `flag.FlagSet`. Packages: `internal/auth`
(config, PKCE, token store, refresh), `internal/api` (HTTP client, Spotify
endpoints, wire→output mapping), `internal/covers` (image cache),
`internal/doctor` (offline report), `internal/output` (output types, `Error`
type with code/message/retry_after, exit-code table, JSON writers).
*Alternative:* cobra/urfave — rejected, ~18 flat commands do not justify a
dependency.

### One error type, one exit path
All layers return `*output.Error` (code, message, optional retry_after);
`main` is the only place that prints errors and calls `os.Exit`, using the
table from the `output-contract` spec. Unknown errors are wrapped as
`internal`. *Alternative:* sentinel errors + switch in each command — more
places to forget a mapping.

### Config format
`config.toml` holds flat `key = "string"` pairs, parsed by a ~30-line parser
(comments, blank lines, quoted strings; anything else is `not_configured`
with a clear message). `SPOTIFY_HELPER_CLIENT_ID` overrides the file.
*Alternatives:* a TOML library (breaks the stdlib-only rule for two keys), or
JSON config (unfriendly to hand-edit and inconsistent with the user's other
tools).

### Token store and refresh locking
`token.json` is written to a temp file in the same directory with mode `0600`
and `rename`d into place. Refresh runs under an exclusive `flock` on a sibling
lock file: after taking the lock the process re-reads the token and refreshes
only if it is still near expiry, so concurrent processes do not rotate the
refresh token twice. The token records the `client_id` it was issued for so a
changed client ID forces a clean `not_authenticated` rather than an obscure
refresh failure. A 401 from the API is mapped to `not_authenticated` without a
replay; proactive refresh (60 s margin) is the only refresh trigger.

### Rate-limit cooldown on disk
On 429 the helper writes `$XDG_CACHE_HOME/spotify-helper/rate-limit.json`
(`{"until_unix":<int>}`, atomic replace, keeping the later of the existing and
new deadline). Every API command checks it first and fails locally with the
remaining seconds. Missing/invalid `Retry-After` → 30 s, minimum 1 s. No
automatic retry. This matters because each invocation is a separate process
and the plugin would otherwise hammer a limited app.

### HTTP client
A thin `api.Client` wraps `net/http` with a 10 s per-request timeout, bearer
auth, base URLs injectable for tests, and a single `do` function that maps
status codes and Spotify's `error.reason` to codes (`NO_ACTIVE_DEVICE`,
`PREMIUM_REQUIRED`) as in the specs. Network errors and timeouts become
`network_error`. 204 on `GET /me/player` means "nothing active".

### Output types decoupled from the wire
Spotify JSON is decoded into private wire structs and mapped to exported
output structs with explicit `json` tags. Shared pieces (`Item`, `Artist`,
`CoverURL(images)`) live in one place so `state`, `queue` and `search` cannot
drift. Nil slices are normalized to `[]` and absent objects to `null`.

### Cover cache
`internal/covers` maps a URL to `<cache>/covers/<first 16 hex of sha256(url)>.jpg`
(the extension is cosmetic; Noctalia sniffs the content). Download uses a
plain `http.Client` with **no Authorization header** and no shared transport
with the Spotify client, so credentials cannot leak to the CDN. The body is
limited with `io.LimitReader` (2 MiB + 1 to detect overflow), content type must
start with `image/`, the file is written to a temp name and renamed. Hits
`Chtimes` the file so "last used" drives eviction. `--covers` collects the URLs
of a finished result, de-duplicates them, downloads with up to 6 workers under
one 8 s context, and fills `cover_path` (`null` on failure); it is a
post-processing step on the output structs, so no command needs its own cover
code. Cleanup (30 days, 100 MiB, LRU) runs at most once per day, gated by a
stamp file mtime, and ignores `ENOENT`.
*Alternatives:* leave covers to the plugin's `noctalia.download` (kept working,
since `cover_url` stays in every output); a per-image `cover` call only
(many process spawns for a 40-item list); always attaching `cover_path`
(would make every `state` poll touch the disk and the CDN).

### Library listing
`playlists` calls `GET /me/playlists`, `albums` calls `GET /me/albums`; both
pass `limit`/`offset` through and map into the same entry types as `search`
(`PlaylistEntry`, `AlbumEntry`). Fields that Spotify may have removed for
Development Mode (for example per-playlist track counts) are deliberately not
part of the contract.

### Doctor
`doctor` reuses the config, token-store and cooldown readers but never calls
the network or the refresh path, and builds its report from plain structs so a
broken config does not stop it. It exits 0 whenever it ran, so a caller can
parse the report instead of the error channel.

### Commands that need a read first
`toggle`, relative `volume`, relative `seek` and `shuffle toggle` /
`repeat cycle` first call `GET /me/player`; that costs one extra request and a
small race window, which is acceptable for a UI helper. Seek clamping uses
`item.duration_ms` from the same snapshot.

### Login flow
Bind `127.0.0.1:<port>` first (fail early if busy), then print the URL on
stderr and try `xdg-open` (best effort, errors ignored). The handler checks
`state` in constant time, answers a small static HTML page, and hands the code
to the main goroutine; invalid callbacks are answered 400 and ignored. A
5-minute context bounds the wait. The verifier is 64 random bytes,
base64url-encoded without padding (86 characters).

### URI handling
One parser accepts `spotify:<type>:<id>` and
`https://open.spotify.com/[intl-xx/]<type>/<id>[?…]`, returns type and
normalized URI, and each command states which types it allows
(`play-uri`: track, episode, album, playlist, artist, show; `queue-add`: track,
episode).

### Testing
Unit tests with `httptest` servers standing in for `accounts.spotify.com` and
`api.spotify.com`, temp dirs for config/cache via the XDG variables, and a
fake clock where time matters (expiry, cooldown). Tests cover each spec
scenario; no real credentials or playback. A short manual smoke checklist with
a real account is part of the tasks because Development Mode behaviour (which
endpoints still exist, search cap) can only be confirmed against Spotify.

## Risks / Trade-offs

- [Spotify removed or changes endpoints for Development Mode apps, including
  `/me/playlists` and `/me/albums`] → early manual smoke test of every endpoint
  (task group 11); map unexpected 403/404 to
  `api_error` with the status in the message rather than guessing.
- [Refresh-token lifetime or rotation rules differ from expectation] → treat
  any rejected refresh as `not_authenticated` and keep the old refresh token
  when none is returned; the user simply runs `login` again.
- [Polling `state` from the plugin trips rate limits] → the cooldown file
  turns a limit into a cheap local failure and surfaces `retry_after` so the
  plugin can back off; recommended polling interval is documented in README.
- [No daemon means TLS + process start per call] → measured cost is far below
  the 24–60 s of the `spotify_player` CLI; revisit only if the plugin needs
  sub-second updates.
- [Hand-written config parser accepts less than full TOML] → documented as a
  strict subset; unsupported syntax fails loudly.
- [`flock` and `xdg-open` tie us to Linux] → accepted, stated in README.
- [The image CDN is slow or unreachable] → `--covers` is opt-in, bounded by one
  8 s deadline and never fails the command; `cover_url` is always present as a
  fallback.
- [Cover cache grows or is read while being written] → atomic rename, 2 MiB
  per file, 100 MiB / 30-day cleanup.

## Open Questions

- Whether Development Mode's `QUOTA_EXCEEDED` 429 reason deserves its own
  error code; deferred until it is observed in practice.
- Whether the plugin wants a `--json-lines`/streaming mode later; not needed
  for the first version.

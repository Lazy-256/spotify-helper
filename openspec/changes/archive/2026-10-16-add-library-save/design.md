## Context

See proposal.md for why. Today `internal/api/library.go` has two GET calls, `internal/cli/library.go`
wires `playlists` and `albums`, and `queue-add` already shows the pattern for "validate a URI,
call Spotify, print `{"ok":true,"uri":…}`" (`spotifyuri.ParseAllowed` with a set of kinds).
`auth.Scopes` is the single list used by `login` (the authorization URL) and by `doctor`
(`scopes_missing` when the stored token lacks one of them). The token file records the scopes
Spotify granted. Spotify's February 2026 API changes replaced the per-type calls (`/me/tracks`,
`/me/albums`, …) by `/me/library` calls that take Spotify URIs, which is what the new commands use.

## Goals / Non-Goals

**Goals:**
- Three small commands that fit the existing pattern and need no new package.
- A caller can tell "log in again" apart from every other failure without parsing messages.

**Non-Goals:**
- A general "library" abstraction over albums, artists and playlists.
- Batching, paging, or caching of `saved` answers (the helper stays stateless between runs).

## Decisions

**1. URI-based endpoints, one item per call.** `save` is `PUT /me/library?uris=<uri>`, `unsave`
is `DELETE /me/library?uris=<uri>`, `saved` is `GET /me/library/contains?uris=<uri>`. They take the
normalized `spotify:<type>:<id>` string, so no type-specific branches and no separate id handling.
Alternative: the older `/me/tracks?ids=` calls, rejected because Spotify is retiring them and the
new ones also cover episodes with the same code.

**2. Accept tracks and episodes only.** The accepted kinds are the same set as `queue-add`
(`spotifyuri.Track`, `spotifyuri.Episode`). The endpoint would take albums, playlists, shows and
artists as well, but their semantics differ (saving an album, following an artist or playlist),
nothing asks for them, and a narrow set keeps `usage` errors easy to explain. Widening it later is
additive.

**3. New error code `scopes_missing`, checked locally before the call.** Alternative: let Spotify
answer 403 and map it. Rejected: the 403 text is not a stable contract, the helper would send a
request it already knows will fail, and a caller could not distinguish a missing scope from any
other 403 (which stays `api_error`). The check reads the stored token's `Scopes` through the same
path that loads the token for the API client and compares with a single required scope. It runs
after argument validation so `usage` errors stay independent of configuration, as for every other
command. The exit code is 3, next to `not_authenticated`, because both are fixed by `login` and
callers already treat exit 3 as "needs setup".

**4. `saved` is a separate command, not a field of `state`.** `state` is polled every few seconds;
an extra `contains` request per poll would double the Web API traffic and raise the chance of
HTTP 429. The caller asks once per track change instead. Alternative: a `--saved` flag on `state`
(one process, but still one more request per poll); can be added later without breaking anything.

**5. `saved` stays on the read scope.** The contains check needs only `user-library-read`, which
every existing token has, so the plugin can show the heart's state before the user has logged in
again and only needs the new scope when the heart is pressed.

**6. Idempotent writes, no read-before-write.** Spotify answers 200 whether or not the item was
already (not) there, and the helper passes that through. Callers that want to toggle call `saved`
first (or trust their own state) and then `save` or `unsave`.

**7. Output shapes follow existing commands.** `save` and `unsave` print `{"ok":true,"uri":…}` like
`queue-add`. `saved` prints `{"uri":…,"saved":bool}` because it returns data, not just an
acknowledgement.

## Risks / Trade-offs

- [Existing users must log in again] → `doctor` already reports `scopes_missing`; README and the
  error message say what to run. Reads (`saved`, `playlists`, `albums`) keep working meanwhile.
- [Development Mode may restrict `/me/library`] → verify by hand with the real account as the
  first manual task; if blocked, fall back to the older `/me/tracks` calls behind the same
  command surface.
- [Spotify's answer to `contains` is a bare array, not an object] → decode strictly: anything but a
  one-element boolean array is `api_error`, never a silent `false`.
- [A just-saved track can read as unsaved for a moment through `contains` due to eventual
  consistency] → callers should trust the result of their own write; noted in the README.

## Migration Plan

1. Land the change; `login` requests the extra scope.
2. The user runs `spotify-helper login` once; `doctor` then reports no problems.
3. Rollback: revert; tokens with the extra scope keep working because extra scopes are harmless.

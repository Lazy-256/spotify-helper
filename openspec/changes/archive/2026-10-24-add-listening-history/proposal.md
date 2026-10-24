## Why

The plugin wants a "Recent" screen (what the user played lately) and a "My top" screen (their top
tracks and artists). Spotify offers both through the Web API, behind two scopes the helper does not
request yet, and neither endpoint is among those Spotify removed for Development Mode in February 2026.
The helper has no command for either.

## What Changes

- `recent`: the user's recently played tracks, newest first, each with the time it was played and the
  context it was played from. Paged by Spotify's time cursors (`--before`, `--after`), at most 50.
- `top tracks` and `top artists`: the user's most listened items for a time range (`--range
  short|medium|long`: about 4 weeks, 6 months, a year), paged with `--limit` and `--offset`.
- Entries reuse the shapes the plugin already knows (`Item` for tracks, the artist entry for artists), so
  every `uri` works with `play-uri`, `queue-add` and the catalog commands. `--covers` works as elsewhere.
- `login` also requests `user-read-recently-played` and `user-top-read`. Unlike the catalog commands,
  these two cannot work without their scope, so they check the stored token first and fail with
  `scopes_missing` ("run `spotify-helper login` again") before sending anything.

**Does not break the JSON contract**: new commands with new outputs. The `login` output lists two more
granted scopes. `doctor` is not changed: a token without the new scopes is not a problem for any other
command.

## Capabilities

### New Capabilities
- `listening-history`: the `recent` and `top` commands, their output shapes, paging, `--covers`, the required scopes and errors.

### Modified Capabilities
- `auth`: `login` requests two more scopes (an added requirement, so this change does not depend on the order of archiving of `add-catalog-browse`, which also touches the scope list).

## Non-goals

- Podcast episodes in the history: Spotify's endpoint does not return them.
- Anything computed from the history (counts, statistics, "on repeat"), persisting it, or merging it with
  the current playback.
- Genres, followers and popularity of top artists and tracks: Spotify deprecated those fields.
- Audio features, recommendations and "related" lists.
- Following artists and the follow scopes (their endpoints changed in February 2026 and need their own check).
- The plugin screens: they are a separate change in `noctalia-spotify`, written after its detail screens
  (`add-detail-screens`), whose list and navigation they reuse.

## Impact

- New `internal/api/history.go`, `internal/cli/history.go` and output types in `internal/output/model.go`;
  `internal/auth/pkce.go` gains two scopes and two constants for `RequireScope`; `internal/covers` is reused.
- `README.md` and `AGENTS.md` are updated in the same commit as the code.
- Requests: `GET /me/player/recently-played` and `GET /me/top/{type}`, one per call.
- Risks: neither endpoint has been called from this app in Development Mode (the first task is a recorded
  check on a real account); a new account may have no history or no top items and gets empty lists; the
  history contains repeats and Spotify keeps only the latest 50 plays; a private listening session is not
  recorded; both lists can be empty without being an error.

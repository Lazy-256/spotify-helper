# covers Specification

## Purpose
Provides cover images as local files, because the Noctalia plugin UI can only load images from disk, with a shared, bounded cache so that repeated calls cost no network traffic.

## Requirements

### Requirement: Cache a single image with `cover`
`spotify-helper cover <url>` SHALL download the image at the given `https://` URL into the cover cache and output `{"ok":true,"url":"<url>","path":"<absolute path>","cached":<bool>}`, where `cached` is `true` when the file already existed and no download took place. The same URL SHALL always map to the same path. A URL that is not `https://`, or missing, SHALL fail with `usage`.

#### Scenario: First download
- **WHEN** the user runs `cover https://i.scdn.co/image/abc` and the image is not cached
- **THEN** the image is stored, the output has `cached` `false`, and `path` points to an existing file

#### Scenario: Second call
- **WHEN** the same command runs again
- **THEN** no network request is made, `cached` is `true` and `path` is the same

#### Scenario: Insecure URL
- **WHEN** the user runs `cover http://example.com/a.jpg`
- **THEN** the command exits 2 with `error.code` `usage`

### Requirement: Download safety
Cover downloads SHALL NOT send any Spotify credentials, SHALL accept only responses whose content type starts with `image/`, and SHALL reject bodies larger than 2 MiB. Files SHALL be written to a temporary name and renamed into place so a concurrent reader never sees a partial file. Cover downloads SHALL NOT start or consult the Spotify rate-limit cooldown.

#### Scenario: Not an image
- **WHEN** the server answers `text/html`
- **THEN** the command exits 1 with `error.code` `api_error` and leaves no file in the cache

#### Scenario: Oversized image
- **WHEN** the body exceeds 2 MiB
- **THEN** the command exits 1 with `error.code` `api_error` and leaves no file in the cache

#### Scenario: No credentials leak
- **WHEN** a cover is downloaded
- **THEN** the request carries no `Authorization` header

### Requirement: `--covers` adds local paths to listings
`state`, `queue`, `search`, `playlists` and `albums` SHALL accept `--covers`. When given, every returned item that has a `cover_url` SHALL also carry `cover_path` (string), and items without a cover or whose download failed SHALL carry `cover_path` `null`. Without the flag, `cover_path` SHALL be absent and no image download SHALL happen. Image downloads for one command SHALL run concurrently, SHALL share a single overall deadline, and a failed or late download SHALL NOT make the command fail. Images already cached SHALL be reused without network access.

#### Scenario: Warm cache
- **WHEN** `state --covers` runs and the current cover is cached
- **THEN** the output item has `cover_path` and no image request is made

#### Scenario: One cover fails
- **WHEN** `search x --covers` runs and one image download fails
- **THEN** the command still exits 0, that entry has `cover_path` `null` and the others have paths

#### Scenario: Flag omitted
- **WHEN** `search x` runs
- **THEN** no entry has a `cover_path` field and no image is downloaded

### Requirement: Cache is bounded
The cover cache directory SHALL be `$XDG_CACHE_HOME/spotify-helper/covers/` (default `~/.cache/…`). The helper SHALL remove files not used for 30 days and SHALL keep the cache within 100 MiB by removing the least recently used files first. Cleanup SHALL run at most once per day and SHALL tolerate files removed by concurrent processes.

#### Scenario: Old file
- **WHEN** cleanup runs and a cached file was last used 31 days ago
- **THEN** it is removed

#### Scenario: Over the cap
- **WHEN** the cache exceeds 100 MiB after a download
- **THEN** the next cleanup removes the least recently used files until it is within the cap

#### Scenario: Concurrent removal
- **WHEN** another process deletes a file during cleanup
- **THEN** cleanup completes without error

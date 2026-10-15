# output-contract Specification

## Purpose
Defines the output, error and exit-code contract shared by every `spotify-helper` command, so that a caller such as the Noctalia plugin can rely on one parsing strategy and one failure model.

## Requirements

### Requirement: Successful output is a single JSON document on stdout
Every command that succeeds SHALL exit with code 0 and write exactly one JSON object followed by a newline to stdout, and SHALL write nothing else to stdout. Commands that have no payload SHALL output `{"ok":true}`. The exceptions are `--help`/`-h`, which print human-readable usage text to stdout and exit 0.

#### Scenario: Command without payload
- **WHEN** `spotify-helper next` succeeds
- **THEN** stdout is exactly `{"ok":true}` followed by a newline and the exit code is 0

#### Scenario: Progress text never reaches stdout
- **WHEN** a command prints informational text for a human (for example the login URL)
- **THEN** that text is written to stderr, not stdout

### Requirement: Failures are a JSON error on stderr
When a command fails it SHALL write nothing to stdout, write exactly one JSON object `{"error":{"code":"<code>","message":"<text>"}}` followed by a newline to stderr, and exit with a non-zero code. `code` SHALL be one of a fixed set of lowercase snake_case identifiers; `message` SHALL be human-readable English and SHALL NOT be relied upon by callers. The error object SHALL be the last line of stderr. Only the interactive `login` command MAY print human-readable lines before it (the authorization URL); every other command writes nothing but the error object to stderr.

#### Scenario: Failure shape
- **WHEN** any command fails
- **THEN** stdout is empty, the last line of stderr is one JSON object with `error.code` and `error.message`, and the exit code is non-zero

#### Scenario: Non-interactive commands keep stderr clean
- **WHEN** a command other than `login` fails
- **THEN** stderr holds exactly one line, the JSON error object

### Requirement: Error codes map to exit codes
The helper SHALL use these error codes and exit codes:

| `error.code` | Exit code |
|---|---|
| `internal`, `api_error` | 1 |
| `usage` | 2 |
| `not_configured`, `not_authenticated` | 3 |
| `no_active_device`, `device_not_found`, `ambiguous_device` | 4 |
| `rate_limited` | 5 |
| `premium_required` | 6 |
| `network_error` | 7 |

#### Scenario: Unknown subcommand
- **WHEN** the user runs `spotify-helper frobnicate`
- **THEN** stderr holds an error with code `usage` and the exit code is 2

#### Scenario: Missing argument
- **WHEN** the user runs `spotify-helper volume` without a value
- **THEN** stderr holds an error with code `usage` and the exit code is 2

### Requirement: Rate limits are reported and respected
When Spotify answers HTTP 429 the helper SHALL fail with code `rate_limited` and an additional integer field `error.retry_after` holding the number of seconds to wait. The value SHALL come from the `Retry-After` header, defaulting to 30 when the header is missing, malformed or negative, and SHALL be at least 1. The helper SHALL record the cooldown on disk so that later invocations, including those started by other processes, fail immediately with `rate_limited` and the remaining seconds without contacting Spotify until the cooldown has passed. The helper SHALL NOT retry a rate-limited request automatically.

#### Scenario: Spotify returns 429
- **WHEN** Spotify answers 429 with `Retry-After: 12`
- **THEN** the command exits 5 with `error.code` `rate_limited` and `error.retry_after` 12, and no second request is made

#### Scenario: Cooldown still active
- **WHEN** a command runs 5 seconds after a 429 with `Retry-After: 12`
- **THEN** it exits 5 with `error.retry_after` between 1 and 7 without sending any request to Spotify

#### Scenario: Cooldown over
- **WHEN** a command runs after the recorded cooldown has passed
- **THEN** it contacts Spotify normally

### Requirement: Network calls are bounded
Every HTTP request to Spotify SHALL have a timeout. A timeout or connection failure SHALL surface as error code `network_error`.

#### Scenario: Spotify does not answer
- **WHEN** Spotify does not respond within the timeout
- **THEN** the command exits 7 with `error.code` `network_error`

### Requirement: Secrets are never printed
The helper SHALL NOT write access tokens, refresh tokens, authorization codes, PKCE verifiers or the configured `client_id` to stdout, stderr (including error messages) or log output. The single exception is the authorization URL that `login` prints to stderr for the user to open, which carries the client ID as a query parameter by necessity.

#### Scenario: Failed token refresh
- **WHEN** a token refresh fails
- **THEN** the error message contains no token value

#### Scenario: Login URL
- **WHEN** `login` prints the authorization URL
- **THEN** it is the only output that contains the client ID, and it goes to stderr

### Requirement: Output field names are a stable contract
Documented JSON field names and types SHALL NOT be renamed, retyped or removed without declaring the change breaking. New fields MAY be added at any time, and callers SHALL ignore fields they do not know.

#### Scenario: Additive change
- **WHEN** a later release adds a field to the `state` output
- **THEN** existing documented fields keep their names and types

### Requirement: Version command
`spotify-helper version` SHALL output `{"version":"<string>"}`.

#### Scenario: Version
- **WHEN** the user runs `spotify-helper version`
- **THEN** stdout is a JSON object with a non-empty string field `version`

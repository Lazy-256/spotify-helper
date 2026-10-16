## MODIFIED Requirements

### Requirement: Error codes map to exit codes
The helper SHALL use these error codes and exit codes:

| `error.code` | Exit code |
|---|---|
| `internal`, `api_error` | 1 |
| `usage` | 2 |
| `not_configured`, `not_authenticated`, `scopes_missing` | 3 |
| `no_active_device`, `device_not_found`, `ambiguous_device` | 4 |
| `rate_limited` | 5 |
| `premium_required` | 6 |
| `network_error` | 7 |

`scopes_missing` means the stored token lacks a scope the command needs; the remedy is to run `spotify-helper login` again.

#### Scenario: Unknown subcommand
- **WHEN** the user runs `spotify-helper frobnicate`
- **THEN** stderr holds an error with code `usage` and the exit code is 2

#### Scenario: Missing argument
- **WHEN** the user runs `spotify-helper volume` without a value
- **THEN** stderr holds an error with code `usage` and the exit code is 2

#### Scenario: Token lacks a needed scope
- **WHEN** a command needs a scope that the stored token does not have
- **THEN** stderr holds an error with code `scopes_missing` and the exit code is 3

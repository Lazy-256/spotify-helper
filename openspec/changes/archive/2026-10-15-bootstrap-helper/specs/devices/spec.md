## Purpose

Lets a caller see the user's available Spotify Connect devices and move playback between them, which is how the helper reaches speakers that no local media interface can see.

## ADDED Requirements

### Requirement: List devices
`spotify-helper devices` SHALL output `{"devices":[…]}` with one object per Spotify Connect device, each with `id` (string or `null` for restricted devices), `name`, `type`, `is_active` (bool), `volume_percent` (int or `null`) and `is_restricted` (bool), in the order Spotify returns them. When no device is available the array SHALL be empty and the exit code 0.

#### Scenario: Two devices
- **WHEN** the account has a speaker (active) and a phone
- **THEN** `devices` has two entries and exactly one has `is_active` `true`

#### Scenario: No devices
- **WHEN** Spotify lists no devices
- **THEN** the output is `{"devices":[]}` and the exit code is 0

### Requirement: Transfer playback
`spotify-helper transfer <device>` SHALL move playback to the device whose `id` equals the argument or, failing that, whose `name` equals it case-insensitively, and output `{"ok":true,"device":{"id":"<id>","name":"<name>"}}`. By default the playing/paused state SHALL be kept; `--play` SHALL make playback start on the target. Restricted devices SHALL NOT be eligible.

#### Scenario: Transfer by name
- **WHEN** the user runs `transfer "Living Room"` and a device with that name exists
- **THEN** playback is transferred to it and the output names that device

#### Scenario: Transfer and start
- **WHEN** the user runs `transfer <id> --play`
- **THEN** playback is transferred and starts on the target

#### Scenario: Unknown device
- **WHEN** no device matches the argument
- **THEN** the command exits 4 with `error.code` `device_not_found`

#### Scenario: Ambiguous name
- **WHEN** two devices share the given name and neither id matches
- **THEN** the command exits 4 with `error.code` `ambiguous_device` and nothing is transferred

#### Scenario: Missing argument
- **WHEN** the user runs `transfer` without an argument
- **THEN** the command exits 2 with `error.code` `usage`

### Requirement: Device errors use the shared mapping
Both commands SHALL use the shared error mapping, including `premium_required` for a 403 `PREMIUM_REQUIRED` answer on transfer.

#### Scenario: Free account transfer
- **WHEN** Spotify answers a transfer with 403 `PREMIUM_REQUIRED`
- **THEN** the command exits 6 with `error.code` `premium_required`

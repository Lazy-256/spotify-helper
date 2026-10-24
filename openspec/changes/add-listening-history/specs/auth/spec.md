## ADDED Requirements

### Requirement: Login also requests the listening-history scopes
`spotify-helper login` SHALL additionally request the scopes `user-read-recently-played` and `user-top-read`, so that `recent` and `top` can work. The granted scopes SHALL be stored and listed in the `login` output like the others. A stored token that lacks them SHALL keep working for every other command, and `doctor` SHALL NOT report them as a problem.

#### Scenario: Authorization URL
- **WHEN** `login` prints the authorization URL
- **THEN** its `scope` parameter contains `user-read-recently-played` and `user-top-read`

#### Scenario: Successful login
- **WHEN** the user approves the request
- **THEN** the output `scopes` includes both new scopes

#### Scenario: An old token elsewhere
- **WHEN** the stored token lacks both scopes and the user runs `state`
- **THEN** the command works and `doctor` reports no `scopes_missing` because of them

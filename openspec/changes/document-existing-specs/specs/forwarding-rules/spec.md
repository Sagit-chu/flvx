## ADDED Requirements

### Requirement: Port Forwarding Rules
The system SHALL support configuring port forwarding rules, defining the listening port on the node and the destination IP/port.

#### Scenario: Rule Configuration
- **WHEN** an admin creates a port forwarding rule
- **THEN** the rule is stored and synchronized to the assigned node.

### Requirement: Rate Limiting
The system SHALL support configuring bandwidth rate limits for tunnels and users.

#### Scenario: Bandwidth Restriction
- **WHEN** a rate limit is applied to a user
- **THEN** their total bandwidth usage does not exceed the specified limit across all their tunnels.

### Requirement: Traffic Accounting
The system MUST track incoming and outgoing traffic volume for each tunnel and user for billing and quota enforcement.

#### Scenario: Traffic Calculation
- **WHEN** traffic flows through a tunnel
- **THEN** the system increments the user's traffic usage counter accurately.

## ADDED Requirements

### Requirement: Tunnel Creation
The system SHALL allow administrators to create tunnels, specifying protocols (TCP, UDP), listening ports, and destination endpoints.

#### Scenario: Create TCP Tunnel
- **WHEN** an admin creates a new TCP tunnel configuration
- **THEN** the backend stores the tunnel definition and assigns it to a node.

### Requirement: Tunnel Forwarding Configuration
The system SHALL support both standard port forwarding (listening on a port and forwarding to a destination) and tunnel forwarding modes.

#### Scenario: Configure Port Forwarding
- **WHEN** configuring a tunnel for port forwarding
- **THEN** traffic arriving at the specified port is forwarded to the destination IP:port.

### Requirement: Tunnel Assignment
The system SHALL allow tunnels to be assigned to specific users, tracking their usage against the user's quota.

#### Scenario: User Tunnel Usage
- **WHEN** a user is assigned a tunnel
- **THEN** traffic passing through that tunnel is accounted for under the user's usage.

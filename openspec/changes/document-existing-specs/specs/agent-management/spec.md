## ADDED Requirements

### Requirement: Agent Registration
The system SHALL require new agents (Nodes) to register using a unique node key/secret.

#### Scenario: Node Connection
- **WHEN** a new agent starts up with a valid configuration
- **THEN** it connects to the backend and is registered as active.

### Requirement: Heartbeat Monitoring
The system SHALL monitor the status of all registered agents using periodic heartbeats.

#### Scenario: Agent Status
- **WHEN** an agent sends periodic heartbeats
- **THEN** the system updates its last-seen timestamp and marks it as online.

### Requirement: Configuration Sync
The system MUST synchronize configuration changes (tunnels, rules) to agents securely and reliably.

#### Scenario: Push Config
- **WHEN** a configuration change is made in the panel
- **THEN** the agent receives the updated configuration via the next heartbeat or push mechanism.

### Requirement: Version Management
The system SHOULD track the version of the agent software running on each node.

#### Scenario: Version Reporting
- **WHEN** an agent connects
- **THEN** it reports its version number to the backend for tracking.

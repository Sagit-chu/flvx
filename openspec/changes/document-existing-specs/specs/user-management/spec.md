## ADDED Requirements

### Requirement: User Registration
The system SHALL allow new users to register an account with a username and password.

#### Scenario: Successful Registration
- **WHEN** a user submits valid registration details
- **THEN** a new user account is created and the user can log in.

### Requirement: User Authentication
The system MUST authenticate users using JWT tokens. The `Authorization` header MUST contain the raw token without a `Bearer` prefix.

#### Scenario: Valid Login
- **WHEN** a user provides correct credentials
- **THEN** the system returns a valid JWT token.

### Requirement: Role Management
The system SHALL support different user roles, specifically Administrator and Regular User, with distinct permissions.

#### Scenario: Admin Access
- **WHEN** an administrator logs in
- **THEN** they have access to system-wide settings and all user management functions.

### Requirement: Resource Quotas
The system SHALL allow administrators to set traffic limits and connection limits for individual users.

#### Scenario: Traffic Limit Enforcement
- **WHEN** a user exceeds their traffic quota
- **THEN** the system prevents further traffic forwarding for that user.

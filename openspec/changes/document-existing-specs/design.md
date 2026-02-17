## Context

FLVX is a distributed system consisting of a central management panel (Backend + Frontend) and multiple forwarding agents (Nodes). The backend manages configuration, users, and billing, while agents handle the actual traffic forwarding using a modified GOST v3 stack. Communication between the panel and agents is secured and synchronized.

## Goals / Non-Goals

**Goals:**
- Document the high-level architecture of the system.
- Describe the data model for users, tunnels, and nodes.
- Explain the communication protocol between Panel and Agent.
- Detail the authentication and authorization mechanisms.

**Non-Goals:**
- Refactoring the existing architecture.
- Detailed code-level documentation of every function.
- Changing the database schema.

## Decisions

- **Architecture**: The system follows a client-server model where the Panel acts as the server and Agents act as clients that pull configuration and push status.
- **Data Model**: Core entities are Users, Nodes (Agents), Tunnels (Groups of rules), and Forwarding Rules.
- **Communication**: Agents use a heartbeat mechanism to report status and fetch configuration updates. The protocol uses AES encryption with a pre-shared key (Node Secret).
- **Authentication**: JWT for Frontend-Backend communication; API Key (Node Secret) for Agent-Backend communication.

## Risks / Trade-offs

- **Security**: The security of the agent communication relies heavily on the secrecy of the Node Secret.
- **Scalability**: Centralized management might become a bottleneck with a very large number of agents.
- **Complexity**: Synchronizing state across distributed agents introduces complexity in handling failures and inconsistencies.

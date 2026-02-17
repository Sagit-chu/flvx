# Project Overview

**Name**: FLVX (Flux Panel)
**Description**: Traffic forwarding management system built on a forked GOST v3 stack. It provides a web-based panel for managing traffic tunnels, users, and forwarding rules.
**Repository**: Monorepo containing Admin API, Web UI, and Forwarding Agent.

## Tech Stack

### Backend (`go-backend/`)
- **Language**: Go
- **Database**: SQLite (default), PostgreSQL (supported)
- **Framework**: Standard library `net/http` (no heavy framework)
- **ORM**: None (Raw SQL via `database/sql`)

### Frontend (`vite-frontend/`)
- **Framework**: React
- **Build Tool**: Vite (using `rolldown-vite` experimental bundler)
- **UI Library**: HeroUI
- **Styling**: Tailwind CSS
- **Mode**: Hybrid (Desktop + Mobile WebView support)

### Agent (`go-gost/`)
- **Language**: Go
- **Base**: Fork of `gost` v3
- **Extensions**: Custom extensions in `go-gost/x/`

### Infrastructure
- **Containerization**: Docker, Docker Compose (v4/v6)
- **CI/CD**: GitHub Actions
- **Installers**: Shell scripts (`panel_install.sh`, `install.sh`)

## Architecture

- **Panel**: Central management server (Go Backend + React Frontend).
- **Agent**: Forwarding node running on remote servers.
- **Communication**:
    - Frontend -> Backend: REST API (JWT Auth, raw token in header).
    - Agent -> Backend: AES-encrypted heartbeat/config sync.

## Conventions

- **Authentication**: `Authorization` header expects raw JWT token (do NOT add `Bearer ` prefix).
- **API Response**: Standard envelope `{code, msg, data, ts}` (code 0 = success).
- **Database**: Backend uses raw SQL queries. Do not introduce an ORM.
- **File Structure**: Flat monorepo with language-prefixed directories (`go-backend`, `go-gost`).
- **Protobuf**: Do not edit generated `.pb.go` files manually.

## Development

- **Backend Build**: `cd go-backend && make build`
- **Frontend Dev**: `cd vite-frontend && npm run dev`
- **Agent Run**: `cd go-gost && go run .`

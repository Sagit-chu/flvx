# PROJECT KNOWLEDGE BASE

**Generated:** Sat Jan 24 2026
**Context:** Monorepo for Flux Panel (Traffic Forwarding)

## OVERVIEW
Flux Panel is a traffic forwarding management system based on [go-gost](https://github.com/go-gost/gost). It manages tunnels, port forwarding, and user quotas.
**Stack:** Monorepo (Java/Spring Boot Backend + React/Vite Frontend + Go/GOST Service).

## STRUCTURE
```
/root/flux-panel/
├── springboot-backend/  # Java 21 + Spring Boot 2.7 Admin API
├── vite-frontend/       # React 18 + Vite + HeroUI/NextUI
├── go-gost/             # Go 1.23 + GOST Extensions (Core logic)
├── docker-compose*.yml  # Deployment configs (v4/v6)
└── *.sh                 # Install scripts (panel_install.sh, install.sh)
```

## WHERE TO LOOK
| Task | Location | Notes |
|------|----------|-------|
| **Admin API** | `springboot-backend/` | Users, quotas, billing logic |
| **UI/Dashboard** | `vite-frontend/` | Management console |
| **Core Forwarding** | `go-gost/` | GOST implementation & extensions |
| **Deploy** | `docker-compose-v4.yml` | Container orchestration |

## CONVENTIONS
- **Monorepo**: 3 distinct languages/stacks. Treat each subdir as a separate project.
- **Docker**: Primary deployment method.
- **Scripts**: `panel_install.sh` for panel, `install.sh` for nodes.

## COMMANDS
```bash
# Quick Deploy (Panel)
./panel_install.sh

# Quick Deploy (Node)
./install.sh

# Docker
docker-compose -f docker-compose-v4.yml up -d
```

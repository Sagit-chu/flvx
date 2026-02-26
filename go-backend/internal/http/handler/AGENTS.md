# BACKEND HTTP HANDLER KNOWLEDGE BASE

**Generated:** Thu Feb 26 2026

## OVERVIEW
HTTP request handlers for FLVX Admin API. Core business logic layer.
**Stack:** Go 1.24, net/http, GORM via Repository pattern.

## STRUCTURE
```
handler/
├── handler.go        # Main Handler struct, login/captcha, job scheduling
├── control_plane.go  # Node control plane API (add/delete/list)
├── federation.go     # Federation/cluster sync API
├── flow_policy.go    # Traffic policy API
├── jobs.go           # Background job management (sync, cleanup)
├── mutations.go      # CRUD for users, tunnels, forwards (~3700 LOC)
└── upgrade.go        # System upgrade API
```

## WHERE TO LOOK
| Task | Location | Notes |
|------|----------|-------|
| **User/Tunnel CRUD** | `mutations.go` | Largest file; all create/update/delete ops |
| **Login/Captcha** | `handler.go` | Login flow, captcha verification |
| **Federation Sync** | `federation.go` | Panel-to-panel sync |
| **Traffic Policies** | `flow_policy.go` | Flow limiting, quota management |
| **Background Jobs** | `jobs.go` | Scheduled sync/cleanup tasks |
| **Node Control** | `control_plane.go` | Node add/delete/list operations |

## CONVENTIONS
- Inherits from parent: GORM via Repository pattern, JWT in Authorization header.
- Large files expected (`mutations.go` ~3700 LOC - central mutation hub).
- Uses `repo.Repository` for DB access via `h.repo.XXX()` methods.
- Handlers never call `repo.DB()` directly — all queries go through Repository methods.
- Domain-driven file split: one file per functional area (federation, jobs, etc.).

## ANTI-PATTERNS
- Do NOT let handlers call `repo.DB()` directly — add a Repository method instead.
- Do NOT change handler signatures without updating router.go.

## COMMANDS
```bash
cd go-backend
go test ./internal/http/handler/...
```

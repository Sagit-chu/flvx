# BACKEND HTTP HANDLER KNOWLEDGE BASE

**Generated:** Sun Feb 15 2026

## OVERVIEW
HTTP request handlers for FLVX Admin API. Core business logic layer.
**Stack:** Go 1.23, net/http, raw SQL (no ORM).

## STRUCTURE
```
handler/
├── handler.go        # Main Handler struct, login/captcha, job scheduling
├── control_plane.go  # Node control plane API (add/delete/list)
├── federation.go     # Federation/cluster sync API
├── flow_policy.go    # Traffic policy API
├── jobs.go           # Background job management (sync, cleanup)
├── mutations.go      # CRUD for users, tunnels, forwards (largest: 100k+ LOC)
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

## CONVENTIONS
- Inherits from parent: raw SQL, no ORM, JWT in Authorization header.
- Large files expected (`mutations.go` 3716 LOC - central mutation hub).
- Uses `sqlite.Repository` for DB access via `repo.XXX()` methods.
- Domain-driven file split: one file per functional area (federation, jobs, etc.).

## ANTI-PATTERNS
- Do NOT add ORM here - uses raw SQL throughout.
- Do NOT change handler signatures without updating router.go.

## COMMANDS
```bash
cd go-backend
go test ./internal/http/handler/...
```

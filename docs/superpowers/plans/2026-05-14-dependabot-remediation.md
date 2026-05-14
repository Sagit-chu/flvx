# Dependabot Remediation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Resolve the open Dependabot dependency alerts for `go-backend`, `vite-frontend`, `go-gost/x`, and `go-gost` without mixing in unrelated business security changes.

**Architecture:** Apply targeted dependency upgrades per module, verify each module before moving to the next, and keep commits scoped to one dependency group. `go-gost/x` is fixed before `go-gost` because the main agent module uses `replace github.com/go-gost/x => ./x`.

**Tech Stack:** Go modules, pnpm, Vite/Rolldown, GitHub CLI Dependabot alerts API.

---

## File Map

- Modify: `go-backend/go.mod`
  Responsibility: update `github.com/jackc/pgx/v5` to the patched version.
- Modify: `go-backend/go.sum`
  Responsibility: reflect Go module checksum changes from the pgx upgrade.
- Modify: `vite-frontend/package.json`
  Responsibility: update direct vulnerable npm dependency versions and configure `pnpm.overrides`.
- Modify: `vite-frontend/pnpm-lock.yaml`
  Responsibility: resolve vulnerable npm transitive dependencies to patched versions.
- Modify: `go-gost/x/go.mod`
  Responsibility: update vulnerable Go dependencies used by the local `github.com/go-gost/x` module.
- Modify: `go-gost/x/go.sum`
  Responsibility: reflect checksum changes for `go-gost/x`.
- Modify: `go-gost/x/dialer/dtls/dialer.go`
  Responsibility: migrate DTLS import path from `github.com/pion/dtls/v2` to `github.com/pion/dtls/v3`.
- Modify: `go-gost/x/listener/dtls/listener.go`
  Responsibility: migrate DTLS import path from `github.com/pion/dtls/v2` to `github.com/pion/dtls/v3`.
- Modify: `go-gost/go.mod`
  Responsibility: sync vulnerable dependency versions for the main agent module while preserving local `replace github.com/go-gost/x => ./x`.
- Modify: `go-gost/go.sum`
  Responsibility: reflect checksum changes for the main agent module.

## Task 1: Capture Baseline Alerts

**Files:**
- Read: GitHub Dependabot alerts API
- Read: `go-backend/go.mod`
- Read: `vite-frontend/package.json`
- Read: `go-gost/x/go.mod`
- Read: `go-gost/go.mod`

- [ ] **Step 1: Query current open Dependabot alerts**

Run:

```bash
gh api 'repos/Sagit-chu/flvx/dependabot/alerts?state=open&per_page=100' --paginate \
  --jq '.[] | [.number,.security_advisory.severity,.dependency.package.ecosystem,.dependency.manifest_path,.dependency.package.name,.security_vulnerability.vulnerable_version_range,(.security_vulnerability.first_patched_version.identifier // "")] | @tsv'
```

Expected: output includes alerts for `github.com/jackc/pgx/v5`, `postcss`, `serialize-javascript`, `fast-uri`, `@babel/plugin-transform-modules-systemjs`, `github.com/sirupsen/logrus`, `github.com/quic-go/quic-go`, `github.com/quic-go/webtransport-go`, and `github.com/pion/dtls/v2`.

- [ ] **Step 2: Confirm starting versions in module manifests**

Run:

```bash
rg -n 'jackc/pgx|postcss|serialize-javascript|pion/dtls|quic-go|webtransport-go|sirupsen/logrus' \
  go-backend/go.mod vite-frontend/package.json go-gost/x/go.mod go-gost/go.mod
```

Expected key lines:

```text
go-backend/go.mod: github.com/jackc/pgx/v5 v5.7.3
vite-frontend/package.json: "postcss": "8.5.6"
vite-frontend/package.json: "serialize-javascript": "7.0.3"
go-gost/x/go.mod: github.com/pion/dtls/v2 v2.2.6
go-gost/x/go.mod: github.com/quic-go/quic-go v0.49.1
go-gost/x/go.mod: github.com/quic-go/webtransport-go v0.8.1-0.20241018022711-4ac2c9250e66
go-gost/x/go.mod: github.com/sirupsen/logrus v1.8.1
```

- [ ] **Step 3: Confirm the DTLS advisory has no patched v2 release**

Run:

```bash
go list -m -versions github.com/pion/dtls/v2
gh api 'advisories/GHSA-9f3f-wv7r-qc8r' --jq '{summary, vulnerabilities}'
```

Expected:

```text
github.com/pion/dtls/v2 ... v2.2.12
```

Expected advisory facts:

```text
github.com/pion/dtls/v2 vulnerable range <= 2.2.12 has no first_patched_version.
github.com/pion/dtls/v3 patched versions include 3.0.11 and 3.1.1.
```

- [ ] **Step 4: Do not commit baseline capture**

Run:

```bash
git status --short
```

Expected: no files are changed by Task 1.

## Task 2: Fix go-backend pgx Alerts

**Files:**
- Modify: `go-backend/go.mod`
- Modify: `go-backend/go.sum`

- [ ] **Step 1: Upgrade pgx to the patched version**

Run:

```bash
(cd go-backend && go get github.com/jackc/pgx/v5@v5.9.2)
```

Expected: `go-backend/go.mod` changes `github.com/jackc/pgx/v5` from `v5.7.3` to `v5.9.2`, and `go-backend/go.sum` updates checksums.

- [ ] **Step 2: Tidy backend module**

Run:

```bash
(cd go-backend && go mod tidy)
```

Expected: command exits with code 0.

- [ ] **Step 3: Verify backend dependency version**

Run:

```bash
rg -n 'github.com/jackc/pgx/v5' go-backend/go.mod
```

Expected:

```text
go-backend/go.mod: github.com/jackc/pgx/v5 v5.9.2
```

- [ ] **Step 4: Run backend tests**

Run:

```bash
(cd go-backend && go test ./...)
```

Expected: all backend packages pass.

- [ ] **Step 5: Commit backend dependency fix**

Run:

```bash
git add go-backend/go.mod go-backend/go.sum
git commit -m "fix: update backend pgx dependency"
```

Expected: one commit containing only `go-backend/go.mod` and `go-backend/go.sum`.

## Task 3: Fix Frontend npm Alerts

**Files:**
- Modify: `vite-frontend/package.json`
- Modify: `vite-frontend/pnpm-lock.yaml`

- [ ] **Step 1: Update direct dependency and pnpm overrides in package.json**

Edit `vite-frontend/package.json` so the relevant entries are exactly:

```json
{
  "devDependencies": {
    "postcss": "8.5.10"
  },
  "pnpm": {
    "overrides": {
      "@babel/plugin-transform-modules-systemjs": "7.29.4",
      "fast-uri": "3.1.2",
      "serialize-javascript": "7.0.5"
    }
  }
}
```

Remove the existing top-level `"overrides"` block after adding `"pnpm.overrides"`. Keep all other existing dependencies and scripts unchanged.

- [ ] **Step 2: Regenerate pnpm lockfile**

Run:

```bash
(cd vite-frontend && pnpm install)
```

Expected: `vite-frontend/pnpm-lock.yaml` updates and install exits with code 0.

- [ ] **Step 3: Verify vulnerable npm versions are absent**

Run:

```bash
rg -n 'postcss@8\.5\.[0-9]:|"postcss":\s*"8\.5\.[0-9]"|serialize-javascript@[0-6]\.|serialize-javascript@7\.0\.[0-4]|fast-uri@3\.1\.[0-1]|plugin-transform-modules-systemjs@7\.29\.[0-3]' vite-frontend/pnpm-lock.yaml vite-frontend/package.json
```

Expected: no output.

- [ ] **Step 4: Verify patched npm versions are present**

Run:

```bash
rg -n 'postcss@8\.5\.10|serialize-javascript@7\.0\.5|fast-uri@3\.1\.2|plugin-transform-modules-systemjs@7\.29\.4' vite-frontend/pnpm-lock.yaml vite-frontend/package.json
```

Expected: output includes patched entries for `postcss@8.5.10`, `serialize-javascript@7.0.5`, `fast-uri@3.1.2`, and `@babel/plugin-transform-modules-systemjs@7.29.4`.

- [ ] **Step 5: Build frontend**

Run:

```bash
(cd vite-frontend && pnpm run build)
```

Expected: TypeScript and Rolldown/Vite build complete successfully.

- [ ] **Step 6: Commit frontend dependency fix**

Run:

```bash
git add vite-frontend/package.json vite-frontend/pnpm-lock.yaml
git commit -m "fix: update frontend vulnerable dependencies"
```

Expected: one commit containing only `vite-frontend/package.json` and `vite-frontend/pnpm-lock.yaml`.

## Task 4: Fix go-gost/x Non-DTLS Alerts

**Files:**
- Modify: `go-gost/x/go.mod`
- Modify: `go-gost/x/go.sum`

- [ ] **Step 1: Upgrade non-DTLS vulnerable Go dependencies**

Run:

```bash
(cd go-gost/x && go get github.com/sirupsen/logrus@v1.8.3 github.com/quic-go/quic-go@v0.57.0 github.com/quic-go/webtransport-go@v0.10.0)
```

Expected: `go-gost/x/go.mod` resolves these dependencies to at least:

```text
github.com/sirupsen/logrus v1.8.3
github.com/quic-go/quic-go v0.57.0
github.com/quic-go/webtransport-go v0.10.0
```

- [ ] **Step 2: Tidy go-gost/x module**

Run:

```bash
(cd go-gost/x && go mod tidy)
```

Expected: command exits with code 0.

- [ ] **Step 3: Verify go-gost/x non-DTLS dependency versions**

Run:

```bash
rg -n 'github.com/sirupsen/logrus|github.com/quic-go/quic-go|github.com/quic-go/webtransport-go' go-gost/x/go.mod
```

Expected output contains versions at or above:

```text
github.com/sirupsen/logrus v1.8.3
github.com/quic-go/quic-go v0.57.0
github.com/quic-go/webtransport-go v0.10.0
```

- [ ] **Step 4: Run go-gost/x tests after non-DTLS upgrades**

Run:

```bash
(cd go-gost/x && go test ./...)
```

Expected: command exits with code 0. If it fails, stop this task before committing and inspect the first compiler error. The only permitted follow-up edits in this task are direct API-compatibility changes in files named by the compiler under `go-gost/x`; rerun this command after each edit.

- [ ] **Step 5: Commit go-gost/x non-DTLS dependency fix**

Run:

```bash
git add go-gost/x/go.mod go-gost/x/go.sum
git commit -m "fix: update gost quic dependencies"
```

Expected: one commit containing `go-gost/x/go.mod` and `go-gost/x/go.sum`, plus only the compiler-named `go-gost/x` files edited during Step 4.

## Task 5: Migrate go-gost/x DTLS From v2 To v3

**Files:**
- Modify: `go-gost/x/go.mod`
- Modify: `go-gost/x/go.sum`
- Modify: `go-gost/x/dialer/dtls/dialer.go`
- Modify: `go-gost/x/listener/dtls/listener.go`

- [ ] **Step 1: Update DTLS imports**

In `go-gost/x/dialer/dtls/dialer.go`, change:

```go
"github.com/pion/dtls/v2"
```

to:

```go
"github.com/pion/dtls/v3"
```

In `go-gost/x/listener/dtls/listener.go`, change:

```go
"github.com/pion/dtls/v2"
```

to:

```go
"github.com/pion/dtls/v3"
```

- [ ] **Step 2: Add patched DTLS v3 module**

Run:

```bash
(cd go-gost/x && go get github.com/pion/dtls/v3@v3.0.11)
```

Expected: `go-gost/x/go.mod` contains `github.com/pion/dtls/v3 v3.0.11` and no longer needs `github.com/pion/dtls/v2`.

- [ ] **Step 3: Tidy and format go-gost/x**

Run:

```bash
(cd go-gost/x && go mod tidy)
gofmt -w go-gost/x/dialer/dtls/dialer.go go-gost/x/listener/dtls/listener.go
```

Expected: command exits with code 0.

- [ ] **Step 4: Verify v2 import and module are removed**

Run:

```bash
rg -n 'github.com/pion/dtls/v2' go-gost/x
rg -n 'github.com/pion/dtls/v3' go-gost/x/go.mod go-gost/x/dialer/dtls/dialer.go go-gost/x/listener/dtls/listener.go
```

Expected:

```text
first command: no output
second command: output includes go.mod, dialer.go, and listener.go
```

- [ ] **Step 5: Run go-gost/x tests after DTLS migration**

Run:

```bash
(cd go-gost/x && go test ./...)
```

Expected: command exits with code 0. If the compiler reports DTLS v3 API errors, edit only `go-gost/x/dialer/dtls/dialer.go` and `go-gost/x/listener/dtls/listener.go`, preserving the existing `dtls.Config`, `dtls.ClientWithContext`, and `dtls.Listen` flow, then rerun this command.

- [ ] **Step 6: Commit DTLS migration**

Run:

```bash
git add go-gost/x/go.mod go-gost/x/go.sum go-gost/x/dialer/dtls/dialer.go go-gost/x/listener/dtls/listener.go
git commit -m "fix: migrate gost dtls dependency"
```

Expected: one commit containing the DTLS import migration and Go module updates.

## Task 6: Sync go-gost Main Module

**Files:**
- Modify: `go-gost/go.mod`
- Modify: `go-gost/go.sum`

- [ ] **Step 1: Upgrade main module vulnerable dependency requirements**

Run:

```bash
(cd go-gost && go get github.com/sirupsen/logrus@v1.8.3 github.com/quic-go/quic-go@v0.57.0 github.com/quic-go/webtransport-go@v0.10.0 github.com/pion/dtls/v3@v3.0.11)
```

Expected: `go-gost/go.mod` resolves vulnerable dependencies to patched versions and preserves this replace directive:

```go
replace github.com/go-gost/x => ./x
```

- [ ] **Step 2: Tidy main agent module**

Run:

```bash
(cd go-gost && go mod tidy)
```

Expected: command exits with code 0.

- [ ] **Step 3: Verify go-gost no longer references vulnerable DTLS v2**

Run:

```bash
rg -n 'github.com/pion/dtls/v2' go-gost/go.mod go-gost/go.sum
rg -n 'github.com/pion/dtls/v3|github.com/quic-go/quic-go|github.com/quic-go/webtransport-go|github.com/sirupsen/logrus|replace github.com/go-gost/x => ./x' go-gost/go.mod
```

Expected:

```text
first command: no output
second command: output includes dtls/v3, quic-go, webtransport-go, logrus, and the local replace directive
```

- [ ] **Step 4: Run go-gost tests**

Run:

```bash
(cd go-gost && go test ./...)
```

Expected: all packages pass.

- [ ] **Step 5: Build go-gost binary**

Run:

```bash
(cd go-gost && go build .)
```

Expected: build exits with code 0.

- [ ] **Step 6: Commit go-gost module sync**

Run:

```bash
git add go-gost/go.mod go-gost/go.sum
git commit -m "fix: sync gost main dependencies"
```

Expected: one commit containing only `go-gost/go.mod` and `go-gost/go.sum`.

## Task 7: Final Dependabot Verification

**Files:**
- Read: GitHub Dependabot alerts API
- Read: Git working tree status

- [ ] **Step 1: Run all verification commands once more**

Run:

```bash
(cd go-backend && go test ./...)
(cd vite-frontend && pnpm run build)
(cd go-gost/x && go test ./...)
(cd go-gost && go test ./...)
(cd go-gost && go build .)
```

Expected: every command exits with code 0.

- [ ] **Step 2: Query open Dependabot alerts after dependency updates**

Run:

```bash
gh api 'repos/Sagit-chu/flvx/dependabot/alerts?state=open&per_page=100' --paginate \
  --jq 'group_by(.security_advisory.severity) | map({severity:.[0].security_advisory.severity,count:length})'
```

Expected: counts are lower than the baseline from Task 1. If Dependabot has not rescanned yet, run the detailed query from Task 1 and confirm the manifest files now contain patched versions locally.

- [ ] **Step 3: Confirm no vulnerable dependency strings remain in manifests**

Run:

```bash
rg -n 'github.com/jackc/pgx/v5 v5\.7\.3|postcss\"\\s*:\\s*\"8\.5\.6|serialize-javascript\"\\s*:\\s*\"7\.0\.3|github.com/pion/dtls/v2|github.com/quic-go/quic-go v0\.49\.1|github.com/quic-go/webtransport-go v0\.8\.1|github.com/sirupsen/logrus v1\.8\.1' \
  go-backend/go.mod vite-frontend/package.json go-gost/x/go.mod go-gost/go.mod
```

Expected: no output.

- [ ] **Step 4: Confirm working tree contains only intentional changes**

Run:

```bash
git status --short
```

Expected: no uncommitted files from this Dependabot remediation remain. Pre-existing unrelated files may still appear; do not stage or revert them.

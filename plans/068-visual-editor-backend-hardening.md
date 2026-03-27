# 068-visual-editor-backend-hardening

## Overview
Close the remaining backend gaps in the visual editor preparation stage by enforcing admin access, hardening the probe API, and replacing the link-test prototype with node-aware diagnostics that match the current frontend flow.

## Task Checklist

### 1. Access Control
- [x] Require admin access for `/api/v1/visual/*` endpoints.

### 2. Probe API
- [x] Refactor node probe handler to use repository methods only.
- [x] Return complete probe data for occupied ports and latest health metrics.

### 3. Link Test API
- [x] Replace host-machine `ping`/`traceroute` execution with node-aware TCP diagnostics.
- [x] Support visual edge requests using source/target node IDs and structured results.

### 4. Frontend Alignment
- [x] Update visual editor frontend to call the hardened APIs with the new request contract.

### 5. Verification
- `go build ./...` in `go-backend` passed.
- `npx tsc --noEmit` in `vite-frontend` passed.
- `npm run build` in `vite-frontend` passed after rerunning outside the sandbox.
- [x] Verify the backend and frontend packages still build after the changes.

# 068-visual-editor-backend-hardening

## Overview
Close the remaining backend gaps in the visual editor preparation stage by enforcing admin access, hardening the probe API, and replacing the link-test prototype with node-aware diagnostics that match the current frontend flow.

## Task Checklist

### 1. Access Control
- [ ] Require admin access for `/api/v1/visual/*` endpoints.

### 2. Probe API
- [ ] Refactor node probe handler to use repository methods only.
- [ ] Return complete probe data for occupied ports and latest health metrics.

### 3. Link Test API
- [ ] Replace host-machine `ping`/`traceroute` execution with node-aware TCP diagnostics.
- [ ] Support visual edge requests using source/target node IDs and structured results.

### 4. Frontend Alignment
- [ ] Update visual editor frontend to call the hardened APIs with the new request contract.

### 5. Verification
- [ ] Verify the backend and frontend packages still build after the changes.

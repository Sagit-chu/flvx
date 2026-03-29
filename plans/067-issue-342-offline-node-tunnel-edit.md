# 067 - Issue #342: Allow Tunnel Edit with Offline Nodes

**Issue:** https://github.com/Sagit-chu/flvx/issues/342

## Problem
When a node goes offline, users cannot edit tunnel configurations at all — including removing the faulty offline node. This creates a deadlock where users must wait for the offline node to recover or manually edit the database.

## Changes Required

### Backend

- [x] 1. **`prepareTunnelCreateState`** (`mutations.go:2800`): Split the offline check into two modes:
  - **Create (excludeTunnelID == 0)**: Keep current behavior — reject any offline non-remote node.
  - **Update (excludeTunnelID > 0)**: Only reject **newly added** offline non-remote nodes. Allow existing offline nodes to remain (they'll be removed or kept). Query existing chain_tunnel records to determine which nodes are "old".

- [x] 2. **`syncForwardServicesWithWarnings`** (`control_plane.go:231`): When a node is offline (sendNodeCommand fails with "节点不在线"), skip it and add a warning instead of returning a hard error. This allows forward rule modifications to succeed partially.

- [x] 3. **`applyTunnelRuntime`** (`mutations.go:3190`): For non-remote local entry nodes, treat offline errors as deferrable (like remote nodes) so tunnel updates don't fail entirely when some nodes are offline.

### Frontend

- [x] 4. **`validateTunnelForm`** (`tunnel/form.ts`): Change validation to only block adding NEW offline nodes. When editing, offline nodes that are being removed should not block submission. Add isEdit parameter to distinguish create vs. edit.

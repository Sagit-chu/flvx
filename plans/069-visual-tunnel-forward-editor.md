# 069-visual-tunnel-forward-editor

## Overview
Extend the existing `/visual` page from a node-topology canvas into a visual editor that can load, edit, and sync the existing tunnel and forward data model by reusing the current `tunnel/*` and `forward/*` APIs.

## Task Checklist

### 1. Context And Data Model
- [x] Audit the current visual page, tunnel page, and forward page request/response contracts.
- [x] Define a combined visual graph model for nodes, tunnels, and forwards.

### 2. Visual Editor Refactor
- [x] Load existing node, tunnel, and forward records into the canvas.
- [x] Support visual selection and editing for tunnels and forwards.
- [x] Preserve and save canvas layout for all visual entities.

### 3. Sync To Existing APIs
- [x] Reuse the existing tunnel create/update APIs for tunnel edits from the visual page.
- [x] Reuse the existing forward create/update APIs for rule edits from the visual page.
- [x] Refresh the visual graph after successful sync so canvas state matches backend state.

### 4. Verification
- [x] Run frontend type/build verification for the visual editor changes.
- [x] Update this checklist as each task is completed.

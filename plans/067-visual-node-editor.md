# 067-visual-node-editor

## Overview
Transform existing list-based forwarding panel into a ComfyUI-style visual node editor using `React Flow`, enabling drag-and-drop multi-level tunnel configurations.

## Task Checklist

### 1. Preparation & Backend Setup
- [x] Define graph storage strategy (Add new table or use `ViteConfig`).
- [x] Create `GET /api/nodes/graph` and `POST /api/nodes/graph` for loading/saving node coordinates.
- [x] Create `POST /api/probe/node/{id}` to fetch live port occupation and health parameters.
- [x] Create `POST /api/link/test` to perform simulated/actual path testing (Ping/Traceroute).

### 2. Frontend Infrastructure
- [x] Install dependencies in `vite-frontend`: `npm install reactflow @reactflow/core recharts`.
- [x] Create the `/visual` page route.
- [x] Scaffold the base `<ReactFlow>` canvas with controls and mini-map.

### 3. Nodes & Edges Customization
- [x] Develop `ServerNode` custom node component with `<Handle>` inputs/outputs.
- [x] Implement node-level visual indicators (Green/Yellow/Red status, metrics).
- [x] Develop custom connection edge displaying protocol markings and traffic direction.

### 4. Interactive Tools
- [x] Implement right-side click Drawer for node full metrics details.
- [x] Implement edge hover tooltip + traceroute/RTT modal on click.
- [x] Hook up WebSocket listeners to push node metric updates into the React Flow state.

### 5. Deployment Core
- [x] Build the serialization logic that walks the React Flow graph (Nodes & Edges).
- [x] Generate backend multi-level tunnel configurations from graph state.
- [x] Implement "Apply to Production" button with validation.ing engine ("Deploy All").

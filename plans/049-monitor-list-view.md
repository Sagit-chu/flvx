# 049 - Monitor List View

## Objective
The user requested that the Monitor page should switch its card-based grid view to a list view similar to a provided screenshot, and a view mode toggle should be added to the top right.

## Expected Features
1. View Mode Toggle
   - Add a state `viewMode` in `pages/monitor.tsx`.
   - Add a toggle button with LayoutGrid/List icons next to the refresh button.
   - Pass `viewMode` down to `MonitorView` component.
2. List View Implementation
   - Extend `MonitorViewProps` with `viewMode: "list" | "grid"`.
   - Render the `ServerCard` grid when `viewMode === "grid"`.
   - Render a `Table` when `viewMode === "list"`.
   - The list view should include:
     - 状态 (Status: Colored dot depending on `isOnline`).
     - 名称 (Name: Node name).
     - 速率 (Speed: Up/Down speeds styled appropriately).
     - 流量 (Traffic: Total Up/Down bytes).
     - 开机时长 (Uptime).
     - 连接数 (Connections: TCP/UDP).
     - CPU (Progress bar).
     - RAM (Progress bar).
     - 存储 (Storage / Disk Progress bar).
     - 操作 (Actions: Eye view icon to open detailed monitor).

## Checklist
- [x] Create plan document.
- [x] Add viewMode state and toggle in `monitor.tsx`.
- [x] Receive viewMode in `MonitorView` and selectively render Grid vs List views.
- [x] Ensure list correctly visualizes node info, speed, traffic, uptime, conns, CPU/RAM/Disk usages, and action icons.
- [x] Fix HeroUI missing TableProps typings (remove `removeWrapper` prop).

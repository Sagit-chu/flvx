# Plan 061: Node Logo By OS Type (Linux Distro)

## Objective
Display different logos for nodes in the 'Monitor - Node - Card/List View' based on their Linux distribution (Ubuntu, Debian, CentOS, Alpine, etc.).

## Tasks
- [x] Agent: Use `gopsutil/v3/host.Info().Platform` to detect the Linux distro and include it in the version string (`distro.go`).
- [x] Agent: Update `main.go` to call `socket.DetectDistro()` instead of `runtime.GOOS`.
- [x] Agent: Ensure version parameter is URL-escaped since it now includes distro info with special chars.
- [x] Backend: Select `version` column in `ListMonitorNodes`.
- [x] Backend: Include `version` in `monitorNodeListItem` JSON response.
- [x] Frontend: Add `version` to TypeScript interfaces (`MonitorNodeApiItem`, `MonitorNode`, `MonitorViewProps`).
- [x] Frontend: Create `distro-icon.tsx` component with SVG logos for Ubuntu, Debian, CentOS/Rocky/Alma, Alpine, Fedora, Arch/Manjaro, and a default Linux (Tux) fallback.
- [x] Frontend: Use `DistroIcon` in `ServerCard` (card view) and list view name column with branded colors per distro.
- [x] All three projects compile cleanly (`go build`, `tsc --noEmit`).

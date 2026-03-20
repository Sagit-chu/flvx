# Tunnel Quality Uptime Kuma Display
**Plan ID**: 056-tunnel-uptime-kuma-bars.md

## Objective
The goal is to modify the "Monitor - Tunnel" UI page to remove the isolated "Quality" column/chip, and replace the specific latency readouts for entry->exit and exit->Bing with an Uptime Kuma style row of visual history bars.

## Steps
- [x] Remove the individual "Quality" column from the tunnel monitor table view.
- [x] Remove the individual "Quality" chip from the tunnel monitor grid view.
- [x] Implement `<UptimeHistoryBar />` to display a historical sequence of up to 30 metrics, coloring by latency (success, warning, danger) and packet loss (danger).
- [x] Modify the front-end to poll/load initial tunnel quality history, effectively padding the history bars instead of them starting empty.
- [x] Update `getMonitorTunnelQuality` auto-polling locally to append strictly to the local history state, truncating appropriately.
- [x] Retain current latency display next to or below the Uptime bars to allow numerical visibility.

## Status
Completed. The interface will now load history for all displayed tunnels upon open, and continue tracking with bars appending in real time every 10-second polling interval.

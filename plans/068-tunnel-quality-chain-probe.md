# PLAN: Detailed Hop-by-Hop Tunnel Quality Probing (Option B)

## Objective
Enhance the tunnel quality monitoring to correctly execute and record hop-by-hop latency and loss through the entire forwarding chain (Entry -> Mids -> Exit), rather than directly forcing Entry to ping Exit. Expose these details in the UI for advanced troubleshooting.

## Tasks

- [ ] **1. DB Schema & Model Updates**
  - Update `model.TunnelQuality` in `model.go` with `ChainDetails string` (`gorm:"column:chain_details;type:text"`).
  - GORM AutoMigrate will handle adding the column to SQLite/PostgreSQL automatically on backend restart.
- [ ] **2. Backend Data Structures (`tunnel_quality_prober.go`)**
  - Define `TunnelQualityHop` to store `FromNodeID`, `FromNodeName`, `ToNodeID`, `ToNodeName`, `Latency`, `Loss`.
  - Update `tunnelQualitySnapshot` to include `ChainDetails []TunnelQualityHop` (`json:"chainDetails,omitempty"`).
  - Update DB query models to pass `ChainDetails` back to the frontend.
- [ ] **3. Prober Logic Restructuring**
  - In `tunnel_quality_prober.go:probeTunnel()`, handle `Type 2` (Forwarding Chain) properly.
  - Extract the intermediate nodes using `splitChainNodeGroups`.
  - Form the hop pairs: `in[0]->mid[0]`, `mid[i]->mid[i+1]`, `mid[last]->out[0]`.
  - Probe each hop sequentially. Resolve target IPs via `resolveChainProbeTarget` using `connect_ip` fields and node preferences.
  - Cumulative metrics: `EntryToExitLatency` = `sum(latency)`. `EntryToExitLoss` = $1 - \prod (1 - loss\_i)$.
- [ ] **4. Frontend API & Component**
  - Add `chainDetails?: string;` to `TunnelQualityApiItem` in `vite-frontend/src/api/types.ts`.
  - In `TunnelMonitorView`, parse the JSON string back into an array of hops if it exists.
  - Design a horizontal topology diagram (e.g., using `heroui/chip` and `lucide-react` arrows) to show `[上海入口] --25ms--> [香港跳板] --15ms--> [落地出口]`.
  - Highlight bottlenecks (e.g., > 100ms or loss > 0%) in yellow or red.

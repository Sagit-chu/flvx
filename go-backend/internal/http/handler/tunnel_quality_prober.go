package handler

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"go-backend/internal/store/model"
)

const (
	tunnelQualityProbeInterval  = 1 * time.Second
	tunnelQualityProbeTimeout   = 8 * time.Second
	tunnelQualityPingTimeoutMs  = 5000
	tunnelQualityRetention      = 24 * time.Hour // keep 24h of history
	tunnelQualityPruneInterval  = 10 * time.Minute
	tunnelQualityReportInterval = 30 * time.Second // DB save interval
)

type TunnelQualityHop struct {
	FromNodeID   int64   `json:"fromNodeId"`
	FromNodeName string  `json:"fromNodeName"`
	ToNodeID     int64   `json:"toNodeId"`
	ToNodeName   string  `json:"toNodeName"`
	Latency      float64 `json:"latency"`
	Loss         float64 `json:"loss"`
	TargetIP     string  `json:"targetIp,omitempty"`
	TargetPort   int     `json:"targetPort,omitempty"`
}

// tunnelQualitySnapshot is the in-memory latest probe result for a tunnel.
type tunnelQualitySnapshot struct {
	TunnelID           int64   `json:"tunnelId"`
	EntryToExitLatency float64 `json:"entryToExitLatency"`
	ExitToBingLatency  float64 `json:"exitToBingLatency"`
	EntryToExitLoss    float64 `json:"entryToExitLoss"`
	ExitToBingLoss     float64 `json:"exitToBingLoss"`
	Success            bool    `json:"success"`
	ErrorMessage       string  `json:"errorMessage,omitempty"`
	Timestamp          int64   `json:"timestamp"`
	ChainDetails       string  `json:"chainDetails,omitempty"`

	// internal fields for db reporting
	lastDBWrite int64 `json:"-"`
}

// tunnelQualityProber runs periodic TCP ping probes against all enabled tunnels.
// Design mirrors health.Checker: background goroutine with worker pool + scheduled cleanup.
type tunnelQualityProber struct {
	handler   *Handler
	cache     sync.Map // tunnelID (int64) → *tunnelQualitySnapshot
	ctx       context.Context
	cancel    context.CancelFunc
	interval  time.Duration
	lastPrune int64
	probing   int32 // atomic flag: 1 = probeAll running, 0 = idle
}

// newTunnelQualityProber creates a new prober (not yet running).
func newTunnelQualityProber(h *Handler) *tunnelQualityProber {
	return &tunnelQualityProber{
		handler:  h,
		interval: tunnelQualityProbeInterval,
	}
}

// Start launches the background probe loop (call from jobs.go).
func (p *tunnelQualityProber) Start(ctx context.Context) {
	// Use the provided context so we stop with other background jobs.
	p.ctx, p.cancel = context.WithCancel(ctx)
	p.loop()
}

// Stop halts the background probe loop.
func (p *tunnelQualityProber) Stop() {
	if p == nil || p.cancel == nil {
		return
	}

	p.cancel()
}

// GetAll returns all cached quality snapshots (latest per tunnel).
func (p *tunnelQualityProber) GetAll() []tunnelQualitySnapshot {
	var items []tunnelQualitySnapshot
	p.cache.Range(func(_, value interface{}) bool {
		if snap, ok := value.(*tunnelQualitySnapshot); ok {
			items = append(items, *snap)
		}
		return true
	})
	return items
}

func (p *tunnelQualityProber) loop() {
	// Initial delay to let the system boot up
	select {
	case <-time.After(5 * time.Second):
	case <-p.ctx.Done():
		return
	}

	// Run once immediately
	p.probeAll()

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			p.probeAll()
			p.maybePrune()
		}
	}
}

func (p *tunnelQualityProber) isEnabled() bool {
	if p == nil || p.handler == nil {
		return true
	}

	return p.handler.isTunnelQualityMonitoringEnabled()
}

// maybePrune deletes old quality rows periodically (mirrors PruneServiceMonitorResults).
func (p *tunnelQualityProber) maybePrune() {
	if !p.isEnabled() {
		return
	}

	now := time.Now().UnixMilli()
	if p.lastPrune > 0 && now-p.lastPrune < int64(tunnelQualityPruneInterval/time.Millisecond) {
		return
	}
	p.lastPrune = now

	h := p.handler
	if h == nil || h.repo == nil {
		return
	}

	cutoff := now - int64(tunnelQualityRetention/time.Millisecond)
	if err := h.repo.PruneTunnelQualityResults(cutoff); err != nil {
		log.Printf("tunnel_quality_prober: prune err=%v", err)
	}
}

func (p *tunnelQualityProber) probeAll() {
	if !p.isEnabled() {
		return
	}

	// Skip if previous probe round is still running (interval < timeout guard)
	if !atomic.CompareAndSwapInt32(&p.probing, 0, 1) {
		return
	}
	defer atomic.StoreInt32(&p.probing, 0)

	h := p.handler
	if h == nil || h.repo == nil {
		return
	}

	tunnelIDs, err := h.repo.ListEnabledTunnelIDs()
	if err != nil {
		log.Printf("tunnel_quality_prober: list enabled tunnels err=%v", err)
		return
	}
	if len(tunnelIDs) == 0 {
		return
	}

	// Probe tunnels concurrently with a worker limit
	// (mirrors health.Checker worker pool pattern)
	const maxWorkers = 20
	sem := make(chan struct{}, maxWorkers)
	var wg sync.WaitGroup

	for _, tunnelID := range tunnelIDs {
		select {
		case <-p.ctx.Done():
			return
		default:
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(tid int64) {
			defer wg.Done()
			defer func() { <-sem }()
			p.probeTunnel(tid)
		}(tunnelID)
	}
	wg.Wait()
}

func (p *tunnelQualityProber) probeTunnel(tunnelID int64) {
	h := p.handler
	if h == nil || h.repo == nil {
		return
	}

	now := time.Now().UnixMilli()
	snap := &tunnelQualitySnapshot{
		TunnelID:  tunnelID,
		Timestamp: now,
	}

	// Get tunnel chain info
	tunnel, err := h.getTunnelRecord(tunnelID)
	if err != nil {
		snap.ErrorMessage = "隧道不存在"
		p.storeResult(snap)
		return
	}

	chainRows, err := h.listChainNodesForTunnel(tunnelID)
	if err != nil || len(chainRows) == 0 {
		snap.ErrorMessage = "隧道配置不完整"
		p.storeResult(snap)
		return
	}

	ipPreference := h.repo.GetTunnelIPPreference(tunnelID)
	inNodes, midNodesGrouped, outNodes := splitChainNodeGroups(chainRows)

	options := diagnosisExecOptions{
		commandTimeout: tunnelQualityProbeTimeout,
		pingTimeoutMS:  tunnelQualityPingTimeoutMs,
		timeoutMessage: "探测超时",
	}

	switch tunnel.Type {
	case 1:
		// Port forwarding: entry → Bing only
		if len(inNodes) > 0 {
			lat, loss, err := p.tcpPingNode(inNodes[0].NodeID, "www.bing.com", 443, options)
			if err == nil {
				snap.ExitToBingLatency = lat
				snap.ExitToBingLoss = loss
				snap.Success = true
			} else {
				snap.ErrorMessage = err.Error()
			}
		}
	case 2:
		// Tunnel forwarding: entry → exit + exit → Bing
		probeOK := true

		if len(inNodes) > 0 && len(outNodes) > 0 {
			var hops []TunnelQualityHop
			var totalLat float64
			remainingSuccessProb := 1.0

			nodesInPath := make([]chainNodeRecord, 0, 2+len(midNodesGrouped))
			nodesInPath = append(nodesInPath, inNodes[0])
			for _, midGroup := range midNodesGrouped {
				if len(midGroup) > 0 {
					nodesInPath = append(nodesInPath, midGroup[0])
				}
			}
			nodesInPath = append(nodesInPath, outNodes[0])

			for i := 0; i < len(nodesInPath)-1; i++ {
				source := nodesInPath[i]
				target := nodesInPath[i+1]

				hop := TunnelQualityHop{
					FromNodeID:   source.NodeID,
					FromNodeName: source.NodeName,
					ToNodeID:     target.NodeID,
					ToNodeName:   target.NodeName,
				}

				targetNode, nodeErr := h.getNodeRecord(target.NodeID)
				if nodeErr != nil || targetNode == nil {
					snap.ErrorMessage = "节点 " + target.NodeName + " 不可用"
					probeOK = false
					hop.Latency = -1
					hop.Loss = 100
					hops = append(hops, hop)
					break
				}
				
				fromNode, _ := h.getNodeRecord(source.NodeID)
				targetIP, targetPort, resolveErr := resolveChainProbeTarget(fromNode, targetNode, target.Port, ipPreference, target.ConnectIP)
				if resolveErr != nil {
					snap.ErrorMessage = "解析节点 " + target.NodeName + " 失败: " + resolveErr.Error()
					probeOK = false
					hop.Latency = -1
					hop.Loss = 100
					hops = append(hops, hop)
					break
				}

				hop.TargetIP = targetIP
				hop.TargetPort = targetPort

				lat, loss, err := p.tcpPingNode(source.NodeID, targetIP, targetPort, options)
				if err == nil {
					hop.Latency = lat
					hop.Loss = loss
					totalLat += lat
					remainingSuccessProb *= (1.0 - loss/100.0)
					hops = append(hops, hop)
				} else {
					probeOK = false
					hop.Latency = -1
					hop.Loss = 100
					hops = append(hops, hop)
					if snap.ErrorMessage == "" {
						snap.ErrorMessage = err.Error()
					}
					break
				}
			}

			if probeOK {
				snap.EntryToExitLatency = totalLat
				snap.EntryToExitLoss = (1.0 - remainingSuccessProb) * 100.0
			} else {
				snap.EntryToExitLatency = -1
				snap.EntryToExitLoss = 100
			}

			if len(hops) > 0 {
				if b, err := json.Marshal(hops); err == nil {
					snap.ChainDetails = string(b)
				}
			}
		}

		// Exit → Bing
		if len(outNodes) > 0 {
			lat, loss, err := p.tcpPingNode(outNodes[0].NodeID, "www.bing.com", 443, options)
			if err == nil {
				snap.ExitToBingLatency = lat
				snap.ExitToBingLoss = loss
			} else {
				if snap.ErrorMessage == "" {
					snap.ErrorMessage = err.Error()
				}
				probeOK = false
			}
		}

		snap.Success = probeOK
	default:
		// Unknown type: entry → Bing
		if len(inNodes) > 0 {
			lat, loss, err := p.tcpPingNode(inNodes[0].NodeID, "www.bing.com", 443, options)
			if err == nil {
				snap.ExitToBingLatency = lat
				snap.ExitToBingLoss = loss
				snap.Success = true
			} else {
				snap.ErrorMessage = err.Error()
			}
		}
	}

	p.storeResult(snap)
}

func (p *tunnelQualityProber) tcpPingNode(nodeID int64, ip string, port int, options diagnosisExecOptions) (latency float64, loss float64, err error) {
	h := p.handler
	if h == nil {
		return 0, 100, nil
	}

	node, nodeErr := h.getNodeRecord(nodeID)
	if nodeErr != nil {
		return 0, 100, nodeErr
	}

	var pingData map[string]interface{}
	var pingErr error
	if node != nil && node.IsRemote == 1 {
		pingData, pingErr = h.tcpPingViaRemoteNode(node, ip, port, options)
	} else {
		pingData, pingErr = h.tcpPingViaNode(nodeID, ip, port, options)
	}
	if pingErr != nil {
		return 0, 100, pingErr
	}

	avgTime := asFloat(pingData["averageTime"], 0)
	packetLoss := asFloat(pingData["packetLoss"], 100)

	return avgTime, packetLoss, nil
}

func (p *tunnelQualityProber) storeResult(snap *tunnelQualitySnapshot) {
	if snap == nil {
		return
	}

	// Update in-memory cache (latest per tunnel)
	// Retain the lastDBWrite timestamp if it exists, so we only DB write every 30s
	var lastWrite int64
	if existing, ok := p.cache.Load(snap.TunnelID); ok {
		if eg, ok := existing.(*tunnelQualitySnapshot); ok {
			lastWrite = eg.lastDBWrite
		}
	}
	snap.lastDBWrite = lastWrite

	now := time.Now().UnixMilli()
	writeToDB := false
	if now-snap.lastDBWrite >= int64(tunnelQualityReportInterval/time.Millisecond) {
		writeToDB = true
		snap.lastDBWrite = now
	}

	p.cache.Store(snap.TunnelID, snap)

	if !writeToDB {
		return
	}

	// Persist to database (history)
	h := p.handler
	if h == nil || h.repo == nil {
		return
	}

	successInt := 0
	if snap.Success {
		successInt = 1
	}

	q := &model.TunnelQuality{
		TunnelID:           snap.TunnelID,
		EntryToExitLatency: snap.EntryToExitLatency,
		ExitToBingLatency:  snap.ExitToBingLatency,
		EntryToExitLoss:    snap.EntryToExitLoss,
		ExitToBingLoss:     snap.ExitToBingLoss,
		Success:            successInt,
		ErrorMessage:       snap.ErrorMessage,
		Timestamp:          snap.Timestamp,
		ChainDetails:       snap.ChainDetails,
	}
	if err := h.repo.InsertTunnelQuality(q); err != nil {
		log.Printf("tunnel_quality_prober: insert db err=%v tunnel_id=%d", err, snap.TunnelID)
	}
}

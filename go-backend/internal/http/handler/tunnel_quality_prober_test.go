package handler

import (
	"fmt"
	"slices"
	"testing"
	"time"
)

func TestTunnelQualityProberUsesConfiguredProbeTarget(t *testing.T) {
	h := setupProbeTargetTunnelHandler(t)
	seedProbeTargetTunnel(t, h, 77, "quality-target", "speed.example.com", 8443)
	if err := h.repo.DB().Exec(`
		INSERT INTO node(id, name, secret, server_ip, server_ip_v4, server_ip_v6, port, interface_name, version, http, tls, socks, created_time, updated_time, status, tcp_listen_addr, udp_listen_addr, inx)
		VALUES(30, 'exit-a', 'exit-secret', '10.0.0.30', '10.0.0.30', '', '30000-30010', '', 'v1', 1, 1, 1, ?, ?, 1, '[::]', '[::]', 0)
	`, time.Now().UnixMilli(), time.Now().UnixMilli()).Error; err != nil {
		t.Fatalf("insert exit node: %v", err)
	}
	if err := h.repo.DB().Exec(`
		INSERT INTO chain_tunnel(tunnel_id, chain_type, node_id, port, strategy, inx, protocol)
		VALUES(77, '3', 30, 30001, 'round', 1, 'tls')
	`).Error; err != nil {
		t.Fatalf("insert exit chain: %v", err)
	}

	p := newTunnelQualityProber(h)
	var calls []string
	p.probeNode = func(nodeID int64, ip string, port int, options diagnosisExecOptions) (float64, float64, error) {
		if options.pingCount != 1 {
			t.Fatalf("expected real-time quality probe count 1, got %d", options.pingCount)
		}
		calls = append(calls, fmt.Sprintf("%d|%s|%d", nodeID, ip, port))
		return 10, 0, nil
	}
	p.probeTunnel(77)

	if !slices.Contains(calls, "10|speed.example.com|8443") {
		t.Fatalf("expected type 1 public probe from entry to configured target, calls=%+v", calls)
	}
	if slices.Contains(calls, "30|speed.example.com|8443") {
		t.Fatalf("did not expect type 1 public probe from exit node, calls=%+v", calls)
	}
	snaps := p.GetAll()
	if len(snaps) != 1 {
		t.Fatalf("expected one quality snapshot, got %+v", snaps)
	}
	if snaps[0].ProbeTargetHost != "speed.example.com" || snaps[0].ProbeTargetPort != 8443 {
		t.Fatalf("unexpected snapshot target metadata: %+v", snaps[0])
	}
}

func TestTunnelQualityProberSkipsAllOfflineExits(t *testing.T) {
	h := setupProbeTargetTunnelHandler(t)
	seedQualityForwardTunnel(t, h, 81, []int{0, 0, 0})

	p := newTunnelQualityProber(h)
	probeCalls := 0
	p.probeNode = func(nodeID int64, ip string, port int, options diagnosisExecOptions) (float64, float64, error) {
		probeCalls++
		return 0, 100, fmt.Errorf("unexpected probe node=%d target=%s:%d", nodeID, ip, port)
	}
	p.probeTunnel(81)

	if probeCalls != 0 {
		t.Fatalf("expected no TCP probes when all exits are offline, got %d", probeCalls)
	}
	snaps := p.GetAll()
	if len(snaps) != 1 {
		t.Fatalf("expected one quality snapshot, got %+v", snaps)
	}
	if snaps[0].Success || snaps[0].ErrorMessage != "出口节点均不在线" {
		t.Fatalf("expected offline exit snapshot, got %+v", snaps[0])
	}
	if snaps[0].EntryToExitLoss != 100 {
		t.Fatalf("expected 100%% entry-to-exit loss, got %+v", snaps[0])
	}
}

func TestTunnelQualityProberUsesOnlineBackupExit(t *testing.T) {
	h := setupProbeTargetTunnelHandler(t)
	seedQualityForwardTunnel(t, h, 82, []int{0, 1})

	p := newTunnelQualityProber(h)
	var calls []string
	p.probeNode = func(nodeID int64, ip string, port int, options diagnosisExecOptions) (float64, float64, error) {
		if options.pingCount != 1 {
			t.Fatalf("expected real-time quality probe count 1, got %d", options.pingCount)
		}
		calls = append(calls, fmt.Sprintf("%d|%s|%d", nodeID, ip, port))
		return 10, 0, nil
	}
	p.probeTunnel(82)

	if slices.Contains(calls, "10|10.0.0.30|30030") {
		t.Fatalf("did not expect probe to offline primary exit, calls=%+v", calls)
	}
	if !slices.Contains(calls, "10|10.0.0.31|30031") {
		t.Fatalf("expected entry probe to online backup exit, calls=%+v", calls)
	}
	if !slices.Contains(calls, "31|www.bing.com|443") {
		t.Fatalf("expected public probe from online backup exit, calls=%+v", calls)
	}
	snaps := p.GetAll()
	if len(snaps) != 1 || !snaps[0].Success {
		t.Fatalf("expected successful backup exit snapshot, got %+v", snaps)
	}
}

func TestTunnelQualityProberStoresProbeTargetWhenChainIncomplete(t *testing.T) {
	h := setupProbeTargetTunnelHandler(t)
	seedProbeTargetTunnel(t, h, 78, "quality-target-incomplete", "speed.example.com", 8443)
	if err := h.repo.DB().Exec(`DELETE FROM chain_tunnel WHERE tunnel_id = ?`, 78).Error; err != nil {
		t.Fatalf("delete chain rows: %v", err)
	}

	p := newTunnelQualityProber(h)
	p.probeTunnel(78)

	snaps := p.GetAll()
	if len(snaps) != 1 {
		t.Fatalf("expected one quality snapshot, got %+v", snaps)
	}
	if snaps[0].ErrorMessage == "" {
		t.Fatalf("expected incomplete chain error, got %+v", snaps[0])
	}
	if snaps[0].ProbeTargetHost != "speed.example.com" || snaps[0].ProbeTargetPort != 8443 {
		t.Fatalf("unexpected snapshot target metadata: %+v", snaps[0])
	}
}

func TestTunnelQualityProberUsesConfiguredInterval(t *testing.T) {
	h := setupProbeTargetTunnelHandler(t)
	if err := h.repo.UpsertConfig("monitor_tunnel_quality_interval_sec", "15", time.Now().UnixMilli()); err != nil {
		t.Fatalf("upsert interval config: %v", err)
	}

	p := newTunnelQualityProber(h)
	if got := p.probeInterval(); got != 15*time.Second {
		t.Fatalf("probe interval = %s, want 15s", got)
	}
}

func TestTunnelQualityProberConfigNotificationIsCoalesced(t *testing.T) {
	p := newTunnelQualityProber(nil)
	p.NotifyConfigChanged()
	p.NotifyConfigChanged()

	if got := len(p.wake); got != 1 {
		t.Fatalf("wake notifications = %d, want 1", got)
	}
}

func TestNormalizeTunnelQualityProbeIntervalConfigValue(t *testing.T) {
	got, err := normalizeAndValidateConfigValue("monitor_tunnel_quality_interval_sec", " 15 ")
	if err != nil || got != "15" {
		t.Fatalf("normalize interval = %q, %v", got, err)
	}
	if _, err := normalizeAndValidateConfigValue("monitor_tunnel_quality_interval_sec", "0"); err == nil {
		t.Fatalf("expected invalid interval to be rejected")
	}
}

func seedQualityForwardTunnel(t *testing.T, h *Handler, tunnelID int64, exitStatuses []int) {
	t.Helper()
	now := time.Now().UnixMilli()
	if err := h.repo.DB().Exec(`
		INSERT INTO tunnel(id, name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, inx, ip_preference, probe_target_host, probe_target_port)
		VALUES(?, ?, 1, 2, 'tls', 1, ?, ?, 1, ?, '', '', 0)
	`, tunnelID, fmt.Sprintf("quality-forward-%d", tunnelID), now, now, tunnelID).Error; err != nil {
		t.Fatalf("insert forwarding tunnel: %v", err)
	}
	if err := h.repo.DB().Exec(`
		INSERT INTO chain_tunnel(tunnel_id, chain_type, node_id, port, strategy, inx, protocol)
		VALUES(?, '1', 10, 30001, 'fifo', 1, 'tls')
	`, tunnelID).Error; err != nil {
		t.Fatalf("insert entry chain: %v", err)
	}
	for i, status := range exitStatuses {
		nodeID := int64(30 + i)
		port := 30030 + i
		ip := fmt.Sprintf("10.0.0.%d", nodeID)
		if err := h.repo.DB().Exec(`
			INSERT INTO node(id, name, secret, server_ip, server_ip_v4, server_ip_v6, port, interface_name, version, http, tls, socks, created_time, updated_time, status, tcp_listen_addr, udp_listen_addr, inx)
			VALUES(?, ?, ?, ?, ?, '', '30000-30100', '', 'v1', 1, 1, 1, ?, ?, ?, '[::]', '[::]', 0)
		`, nodeID, fmt.Sprintf("exit-%d", i+1), fmt.Sprintf("exit-secret-%d", i+1), ip, ip, now, now, status).Error; err != nil {
			t.Fatalf("insert exit node %d: %v", nodeID, err)
		}
		if err := h.repo.DB().Exec(`
			INSERT INTO chain_tunnel(tunnel_id, chain_type, node_id, port, strategy, inx, protocol)
			VALUES(?, '3', ?, ?, 'fifo', ?, 'tls')
		`, tunnelID, nodeID, port, i+1).Error; err != nil {
			t.Fatalf("insert exit chain %d: %v", nodeID, err)
		}
	}
}

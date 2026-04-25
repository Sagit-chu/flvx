package handler

import (
	"testing"
	"time"

	"go-backend/internal/store/model"
	"go-backend/internal/store/repo"
)

func TestBuildForwardServiceConfigsPreservesProxyProtocolWithInterfaceMetadata(t *testing.T) {
	forward := &forwardRecord{
		ID:            1,
		UserID:        2,
		TunnelID:      3,
		RemoteAddr:    "1.1.1.1:443",
		Strategy:      "fifo",
		ProxyProtocol: 2,
	}
	tunnel := &tunnelRecord{Type: 1}
	node := &nodeRecord{
		InterfaceName: "eth0",
		TCPListenAddr: "0.0.0.0",
		UDPListenAddr: "0.0.0.0",
	}

	services := buildForwardServiceConfigs("1_2_3", forward, tunnel, node, 4001, "", nil, "")
	if len(services) != 2 {
		t.Fatalf("expected 2 services, got %d", len(services))
	}

	for _, service := range services {
		metadata, ok := service["metadata"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected metadata map, got %T", service["metadata"])
		}
		if metadata["interface"] != "eth0" {
			t.Fatalf("expected interface metadata eth0, got %v", metadata["interface"])
		}
		if metadata["proxyProtocol"] != 2 {
			t.Fatalf("expected proxyProtocol 2, got %v", metadata["proxyProtocol"])
		}
	}
}

func TestRollbackForwardMutationRestoresProxyProtocol(t *testing.T) {
	r, err := repo.Open(":memory:")
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	now := time.Now().UnixMilli()
	if err := r.DB().Create(&model.Forward{
		UserID:        2,
		UserName:      "rollback-user",
		Name:          "rollback-forward",
		TunnelID:      3,
		RemoteAddr:    "9.9.9.9:443",
		Strategy:      "fifo",
		CreatedTime:   now,
		UpdatedTime:   now,
		Status:        1,
		ProxyProtocol: 2,
	}).Error; err != nil {
		t.Fatalf("create forward: %v", err)
	}

	forwardID := mustLastInsertID(t, r, "rollback-forward")
	if err := r.DB().Model(&model.Forward{}).Where("id = ?", forwardID).Updates(map[string]interface{}{
		"name":           "changed-forward",
		"proxy_protocol": 0,
		"updated_time":   now + 1,
	}).Error; err != nil {
		t.Fatalf("mutate forward: %v", err)
	}

	h := &Handler{repo: r}
	h.rollbackForwardMutation(&forwardRecord{
		ID:            forwardID,
		UserID:        2,
		UserName:      "rollback-user",
		Name:          "rollback-forward",
		TunnelID:      3,
		RemoteAddr:    "9.9.9.9:443",
		Strategy:      "fifo",
		Status:        1,
		ProxyProtocol: 2,
	}, nil)

	var proxyProtocol int
	if err := r.DB().Raw("SELECT proxy_protocol FROM forward WHERE id = ?", forwardID).Row().Scan(&proxyProtocol); err != nil {
		t.Fatalf("query proxy_protocol: %v", err)
	}
	if proxyProtocol != 2 {
		t.Fatalf("expected proxyProtocol restored to 2, got %d", proxyProtocol)
	}
}

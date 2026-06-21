package handler

import (
	"database/sql"
	"testing"
	"time"

	"go-backend/internal/store/model"
	"go-backend/internal/store/repo"
)

func TestBuildForwardServiceConfigsAppliesProxyProtocolReceiveAndSendIndependently(t *testing.T) {
	forward := &forwardRecord{
		ID:                   1,
		UserID:               2,
		TunnelID:             3,
		RemoteAddr:           "1.1.1.1:443",
		Strategy:             "fifo",
		ProxyProtocolReceive: 1,
		ProxyProtocolSend:    2,
	}
	tunnel := &tunnelRecord{Type: 1}
	node := &nodeRecord{
		InterfaceName: "eth0",
		TCPListenAddr: "0.0.0.0",
		UDPListenAddr: "0.0.0.0",
	}

	services := buildForwardServiceConfigs("1_2_3", forward, tunnel, node, 4001, "", forwardRuntimeLimiters{})
	if len(services) != 2 {
		t.Fatalf("expected 2 services, got %d", len(services))
	}

	for _, service := range services {
		serviceMetadata, ok := service["metadata"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected metadata map, got %T", service["metadata"])
		}
		if serviceMetadata["interface"] != "eth0" {
			t.Fatalf("expected interface metadata eth0, got %v", serviceMetadata["interface"])
		}
		if serviceMetadata["proxyProtocol"] != 1 {
			t.Fatalf("expected service proxyProtocol 1 for receive mode, got %v", serviceMetadata["proxyProtocol"])
		}

		handlerConfig, ok := service["handler"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected handler config map, got %T", service["handler"])
		}
		handlerMetadata, ok := handlerConfig["metadata"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected handler metadata map, got %T", handlerConfig["metadata"])
		}
		if handlerMetadata["proxyProtocol"] != 2 {
			t.Fatalf("expected handler proxyProtocol 2, got %v", handlerMetadata["proxyProtocol"])
		}
	}
}

func TestBuildForwardServiceConfigsKeepsLegacyProxyProtocolAsSend(t *testing.T) {
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
		TCPListenAddr: "0.0.0.0",
		UDPListenAddr: "0.0.0.0",
	}

	services := buildForwardServiceConfigs("1_2_3", forward, tunnel, node, 4001, "", forwardRuntimeLimiters{})
	if len(services) != 2 {
		t.Fatalf("expected 2 services, got %d", len(services))
	}

	for _, service := range services {
		serviceMetadata, _ := service["metadata"].(map[string]interface{})
		if _, ok := serviceMetadata["proxyProtocol"]; ok {
			t.Fatalf("legacy proxyProtocol should not enable receive mode: %v", serviceMetadata)
		}

		handlerConfig, ok := service["handler"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected handler config map, got %T", service["handler"])
		}
		handlerMetadata, ok := handlerConfig["metadata"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected handler metadata map, got %T", handlerConfig["metadata"])
		}
		if handlerMetadata["proxyProtocol"] != 2 {
			t.Fatalf("expected legacy proxyProtocol to send version 2, got %v", handlerMetadata["proxyProtocol"])
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
		IPMaxConn:     5,
		IPSpeedID:     sql.NullInt64{Int64: 21, Valid: true},
		ProxyProtocol: 2,
	}).Error; err != nil {
		t.Fatalf("create forward: %v", err)
	}

	forwardID := mustLastInsertID(t, r, "rollback-forward")
	if err := r.DB().Model(&model.Forward{}).Where("id = ?", forwardID).Updates(map[string]interface{}{
		"name":           "changed-forward",
		"ip_max_conn":    0,
		"ip_speed_id":    nil,
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
		IPMaxConn:     5,
		IPSpeedID:     sql.NullInt64{Int64: 21, Valid: true},
		ProxyProtocol: 2,
	}, nil)

	var record model.Forward
	if err := r.DB().Where("id = ?", forwardID).First(&record).Error; err != nil {
		t.Fatalf("query forward: %v", err)
	}
	if record.ProxyProtocol != 2 {
		t.Fatalf("expected proxyProtocol restored to 2, got %d", record.ProxyProtocol)
	}
	if record.IPMaxConn != 5 {
		t.Fatalf("expected ipMaxConn restored to 5, got %d", record.IPMaxConn)
	}
	if !record.IPSpeedID.Valid || record.IPSpeedID.Int64 != 21 {
		t.Fatalf("expected ipSpeedId restored to 21, got %+v", record.IPSpeedID)
	}
}

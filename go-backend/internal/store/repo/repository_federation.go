package repo

import (
	"database/sql"
	"errors"

	"go-backend/internal/store/model"

	"gorm.io/gorm"
)

// RemoteNodeRow holds the columns fetched for a remote node listing.
type RemoteNodeRow struct {
	ID           int64
	Name         string
	RemoteURL    sql.NullString
	RemoteToken  sql.NullString
	RemoteConfig sql.NullString
}

// NodeBasicInfo holds name, server_ip, and status for a node.
type NodeBasicInfo struct {
	Name     string
	ServerIP string
	Status   int
}

// FederationBindingRow holds the columns for an active federation tunnel binding.
type FederationBindingRow struct {
	ID              int64
	TunnelID        int64
	TunnelName      string
	ChainType       int
	HopInx          int
	AllocatedPort   int
	ResourceKey     string
	RemoteBindingID string
	UpdatedTime     int64
}

type ActiveForwardPortRow struct {
	ForwardID   int64
	TunnelID    int64
	TunnelName  string
	Port        int
	UpdatedTime int64
}

// ListRemoteNodes returns all nodes with is_remote=1, ordered by id desc.
func (r *Repository) ListRemoteNodes() ([]RemoteNodeRow, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var result []RemoteNodeRow
	err := r.db.Model(&model.Node{}).
		Select("id, name, remote_url, remote_token, remote_config").
		Where("is_remote = 1").
		Order("id DESC").
		Find(&result).Error
	if err != nil {
		return nil, err
	}
	if result == nil {
		result = make([]RemoteNodeRow, 0)
	}
	return result, nil
}

// UpdateNodeRemoteConfig sets the remote_config JSON for a given node.
func (r *Repository) UpdateNodeRemoteConfig(nodeID int64, configJSON string) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	return r.db.Model(&model.Node{}).Where("id = ?", nodeID).Update("remote_config", configJSON).Error
}

// ListActiveBindingsForNode returns active federation tunnel bindings for a node.
func (r *Repository) ListActiveBindingsForNode(nodeID int64) ([]FederationBindingRow, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var result []FederationBindingRow
	err := r.db.Model(&model.FederationTunnelBinding{}).
		Select("federation_tunnel_binding.id, federation_tunnel_binding.tunnel_id, COALESCE(tunnel.name, '') AS tunnel_name, federation_tunnel_binding.chain_type, federation_tunnel_binding.hop_inx, federation_tunnel_binding.allocated_port, federation_tunnel_binding.resource_key, federation_tunnel_binding.remote_binding_id, federation_tunnel_binding.updated_time").
		Joins("LEFT JOIN tunnel ON tunnel.id = federation_tunnel_binding.tunnel_id").
		Where("federation_tunnel_binding.node_id = ? AND federation_tunnel_binding.status = 1", nodeID).
		Order("federation_tunnel_binding.allocated_port ASC, federation_tunnel_binding.id ASC").
		Find(&result).Error
	if err != nil {
		return nil, err
	}
	if result == nil {
		result = make([]FederationBindingRow, 0)
	}
	return result, nil
}

func (r *Repository) ListActiveForwardPortsForNode(nodeID int64) ([]ActiveForwardPortRow, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var result []ActiveForwardPortRow
	err := r.db.Model(&model.ForwardPort{}).
		Select("forward_port.forward_id, forward.tunnel_id, COALESCE(tunnel.name, '') AS tunnel_name, forward_port.port, forward.updated_time").
		Joins("JOIN forward ON forward.id = forward_port.forward_id").
		Joins("LEFT JOIN tunnel ON tunnel.id = forward.tunnel_id").
		Where("forward_port.node_id = ? AND forward_port.port > 0", nodeID).
		Order("forward_port.port ASC, forward_port.id ASC").
		Find(&result).Error
	if err != nil {
		return nil, err
	}
	if result == nil {
		result = make([]ActiveForwardPortRow, 0)
	}
	return result, nil
}

// GetNodeBasicInfo returns the name, server_ip, and status for a given node.
func (r *Repository) GetNodeBasicInfo(nodeID int64) (*NodeBasicInfo, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var n model.Node
	err := r.db.Select("name", "server_ip", "status").Where("id = ?", nodeID).First(&n).Error
	if err != nil {
		return nil, err
	}
	return &NodeBasicInfo{Name: n.Name, ServerIP: n.ServerIP, Status: n.Status}, nil
}

// CreateFederationTunnel creates a tunnel and chain_tunnel entry in a transaction,
// returning the new tunnel ID.
func (r *Repository) CreateFederationTunnel(name string, tunnelType int, protocol string, now int64, nodeID int64, remotePort int) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("repository not initialized")
	}
	tunnel := model.Tunnel{
		Name:        name,
		Type:        tunnelType,
		Protocol:    protocol,
		Flow:        0,
		CreatedTime: now,
		UpdatedTime: now,
		Status:      1,
		InIP:        sql.NullString{String: "", Valid: false},
	}
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&tunnel).Error; err != nil {
			return err
		}
		ct := model.ChainTunnel{
			TunnelID:  tunnel.ID,
			ChainType: "1",
			NodeID:    nodeID,
			Port:      sql.NullInt64{Int64: int64(remotePort), Valid: true},
			Strategy:  sql.NullString{String: "fifo", Valid: true},
			Inx:       sql.NullInt64{Int64: 0, Valid: true},
			Protocol:  sql.NullString{String: protocol, Valid: true},
		}
		if err := tx.Create(&ct).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return tunnel.ID, nil
}

// ListUsedPortsOnNode returns all ports in use on a given node from chain_tunnel and forward_port tables.
func (r *Repository) ListUsedPortsOnNode(nodeID int64) ([]int, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	used := make(map[int]struct{})

	var chainPorts []int
	err := r.db.Model(&model.ChainTunnel{}).
		Where("node_id = ? AND port > 0", nodeID).
		Pluck("port", &chainPorts).Error
	if err != nil {
		return nil, err
	}
	for _, p := range chainPorts {
		if p > 0 {
			used[p] = struct{}{}
		}
	}

	var forwardPorts []int
	err = r.db.Model(&model.ForwardPort{}).
		Where("node_id = ? AND port > 0", nodeID).
		Pluck("port", &forwardPorts).Error
	if err != nil {
		return nil, err
	}
	for _, p := range forwardPorts {
		if p > 0 {
			used[p] = struct{}{}
		}
	}

	result := make([]int, 0, len(used))
	for p := range used {
		result = append(result, p)
	}
	return result, nil
}

// ListTunnelIDsByNamePrefix returns all tunnel IDs whose name starts with the given prefix.
func (r *Repository) ListTunnelIDsByNamePrefix(prefix string) ([]int64, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var ids []int64
	err := r.db.Model(&model.Tunnel{}).
		Where("name LIKE ?", prefix+"%").
		Order("id ASC").
		Pluck("id", &ids).Error
	if err != nil {
		return nil, err
	}
	if ids == nil {
		ids = make([]int64, 0)
	}
	return ids, nil
}

// NextIndex returns COALESCE(MAX(inx), -1) + 1 for the given table.
func (r *Repository) NextIndex(table string) int {
	if r == nil || r.db == nil {
		return 0
	}
	var modelRef interface{}
	switch table {
	case "node":
		modelRef = &model.Node{}
	case "tunnel":
		modelRef = &model.Tunnel{}
	case "forward":
		modelRef = &model.Forward{}
	default:
		return 0
	}

	type inxRow struct {
		Inx int
	}
	var row inxRow
	err := r.db.Model(modelRef).
		Select("inx").
		Order("inx DESC").
		Limit(1).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0
	}
	if err != nil {
		return 0
	}
	if row.Inx < 0 {
		return 0
	}
	return row.Inx + 1
}

// CreateRemoteNode inserts a new remote node.
func (r *Repository) CreateRemoteNode(name, secret, serverIP, portRange string, now int64, status int, inx int, remoteURL, remoteToken, remoteConfigJSON string) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	node := model.Node{
		Name:          name,
		Secret:        secret,
		ServerIP:      serverIP,
		ServerIPV4:    sql.NullString{},
		ServerIPV6:    sql.NullString{},
		Port:          portRange,
		InterfaceName: sql.NullString{},
		Version:       sql.NullString{},
		HTTP:          0,
		TLS:           0,
		Socks:         0,
		CreatedTime:   now,
		UpdatedTime:   sql.NullInt64{Int64: now, Valid: true},
		Status:        status,
		TCPListenAddr: "[::]",
		UDPListenAddr: "[::]",
		Inx:           inx,
		IsRemote:      1,
		RemoteURL:     sql.NullString{String: remoteURL, Valid: remoteURL != ""},
		RemoteToken:   sql.NullString{String: remoteToken, Valid: remoteToken != ""},
		RemoteConfig:  sql.NullString{String: remoteConfigJSON, Valid: remoteConfigJSON != ""},
	}
	return r.db.Create(&node).Error
}

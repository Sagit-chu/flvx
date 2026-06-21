package repo

import (
	"database/sql"
	"errors"
	"strings"

	"go-backend/internal/store/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type NftSSHConfigInput struct {
	Host       string
	Port       int
	Username   string
	AuthType   string
	Password   string
	PrivateKey string
	Passphrase string
	SudoMode   string
}

type NftRuleBindingInput struct {
	ForwardID  int64
	NodeID     int64
	InPort     int
	Protocols  string
	TargetAddr string
	BindIP     string
	RuleHash   string
	Status     string
	LastError  string
}

func (r *Repository) UpsertNodeSSHConfig(nodeID int64, cfg NftSSHConfigInput, now int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	if nodeID <= 0 {
		return errors.New("node id is required")
	}
	port := cfg.Port
	if port <= 0 {
		port = 22
	}
	authType := strings.TrimSpace(strings.ToLower(cfg.AuthType))
	if authType == "" {
		authType = "private_key"
	}
	sudoMode := strings.TrimSpace(strings.ToLower(cfg.SudoMode))
	if sudoMode == "" {
		sudoMode = "none"
	}
	row := model.NodeSSHConfig{
		NodeID:      nodeID,
		Host:        strings.TrimSpace(cfg.Host),
		Port:        port,
		Username:    strings.TrimSpace(cfg.Username),
		AuthType:    authType,
		Password:    nullStringFromInterface(cfg.Password),
		PrivateKey:  nullStringFromInterface(cfg.PrivateKey),
		Passphrase:  nullStringFromInterface(cfg.Passphrase),
		SudoMode:    sudoMode,
		CreatedTime: now,
		UpdatedTime: now,
	}
	return r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "node_id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"host":         row.Host,
			"port":         row.Port,
			"username":     row.Username,
			"auth_type":    row.AuthType,
			"password":     row.Password,
			"private_key":  row.PrivateKey,
			"passphrase":   row.Passphrase,
			"sudo_mode":    row.SudoMode,
			"updated_time": row.UpdatedTime,
		}),
	}).Create(&row).Error
}

func (r *Repository) GetNodeSSHConfig(nodeID int64) (*model.NodeSSHConfig, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	return r.GetNodeSSHConfigTx(r.db, nodeID)
}

func (r *Repository) GetNodeSSHConfigTx(tx *gorm.DB, nodeID int64) (*model.NodeSSHConfig, error) {
	if tx == nil {
		return nil, errors.New("database unavailable")
	}
	var cfg model.NodeSSHConfig
	if err := tx.Where("node_id = ?", nodeID).First(&cfg).Error; err != nil {
		return nil, normalizeNotFoundErr(err)
	}
	return &cfg, nil
}

func (r *Repository) DeleteNodeSSHConfig(nodeID int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	return r.db.Where("node_id = ?", nodeID).Delete(&model.NodeSSHConfig{}).Error
}

func (r *Repository) UpsertNftRuleBinding(input NftRuleBindingInput, now int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	row := model.NftRuleBinding{
		ForwardID:   input.ForwardID,
		NodeID:      input.NodeID,
		InPort:      input.InPort,
		Protocols:   defaultString(strings.TrimSpace(input.Protocols), "tcp,udp"),
		TargetAddr:  strings.TrimSpace(input.TargetAddr),
		BindIP:      strings.TrimSpace(input.BindIP),
		RuleHash:    strings.TrimSpace(input.RuleHash),
		Status:      defaultString(strings.TrimSpace(input.Status), "pending"),
		LastError:   strings.TrimSpace(input.LastError),
		AppliedTime: now,
		CreatedTime: now,
		UpdatedTime: now,
	}
	return r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "forward_id"}, {Name: "node_id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"in_port":      row.InPort,
			"protocols":    row.Protocols,
			"target_addr":  row.TargetAddr,
			"bind_ip":      row.BindIP,
			"rule_hash":    row.RuleHash,
			"status":       row.Status,
			"last_error":   row.LastError,
			"applied_time": row.AppliedTime,
			"updated_time": row.UpdatedTime,
		}),
	}).Create(&row).Error
}

func (r *Repository) MarkNftRuleBindingError(forwardID, nodeID int64, message string, now int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	return r.db.Model(&model.NftRuleBinding{}).
		Where("forward_id = ? AND node_id = ?", forwardID, nodeID).
		Updates(map[string]interface{}{
			"status":       "error",
			"last_error":   strings.TrimSpace(message),
			"updated_time": now,
		}).Error
}

func (r *Repository) ListNftRuleBindingsByNode(nodeID int64) ([]model.NftRuleBinding, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var rows []model.NftRuleBinding
	err := r.db.Where("node_id = ?", nodeID).Order("forward_id ASC").Find(&rows).Error
	return rows, err
}

func (r *Repository) DeleteNftRuleBindingsByForward(forwardID int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	return r.db.Where("forward_id = ?", forwardID).Delete(&model.NftRuleBinding{}).Error
}

func (r *Repository) GetNodeForwardMode(nodeID int64) (string, error) {
	if r == nil || r.db == nil {
		return "", errors.New("repository not initialized")
	}
	return r.GetNodeForwardModeTx(r.db, nodeID)
}

func (r *Repository) GetNodeForwardModeTx(tx *gorm.DB, nodeID int64) (string, error) {
	if tx == nil {
		return "", errors.New("database unavailable")
	}
	var row struct {
		ForwardMode sql.NullString `gorm:"column:forward_mode"`
	}
	err := tx.Model(&model.Node{}).Select("forward_mode").Where("id = ?", nodeID).First(&row).Error
	if err != nil {
		return "", normalizeNotFoundErr(err)
	}
	return defaultNodeForwardMode(row.ForwardMode.String), nil
}

func (r *Repository) ListActiveForwardsByNode(nodeID int64) ([]model.ForwardRecord, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var forwards []model.Forward
	err := r.db.Model(&model.Forward{}).
		Joins("JOIN forward_port ON forward_port.forward_id = forward.id").
		Where("forward_port.node_id = ? AND forward.status = 1", nodeID).
		Order("forward.id ASC").
		Distinct("forward.*").
		Find(&forwards).Error
	if err != nil {
		return nil, err
	}
	rows := make([]model.ForwardRecord, 0, len(forwards))
	for _, f := range forwards {
		proxyProtocolReceive, proxyProtocolSend := normalizeForwardProxyProtocol(f.ProxyProtocol, f.ProxyProtocolReceive, f.ProxyProtocolSend)
		rows = append(rows, model.ForwardRecord{
			ID:                   f.ID,
			UserID:               f.UserID,
			UserName:             f.UserName,
			Name:                 f.Name,
			TunnelID:             f.TunnelID,
			RemoteAddr:           f.RemoteAddr,
			Strategy:             f.Strategy,
			Status:               f.Status,
			SpeedID:              f.SpeedID,
			MaxConn:              f.MaxConn,
			IPMaxConn:            f.IPMaxConn,
			IPSpeedID:            f.IPSpeedID,
			ProxyProtocol:        f.ProxyProtocol,
			ProxyProtocolReceive: proxyProtocolReceive,
			ProxyProtocolSend:    proxyProtocolSend,
		})
	}
	for i := range rows {
		if strings.TrimSpace(rows[i].Strategy) == "" {
			rows[i].Strategy = "fifo"
		}
	}
	return rows, nil
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func normalizeForwardProxyProtocol(legacy, receive, send int) (int, int) {
	if send == 0 && legacy > 0 {
		send = legacy
	}
	return receive, send
}

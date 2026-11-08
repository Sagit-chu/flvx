package repo

import (
	"errors"
	"go-backend/internal/relaysecurity"
	"go-backend/internal/store/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// RelayEndpointCredentials loads existing state, creating only when explicitly
// requested by a new endpoint operation. Reconnects must never recreate a secret.
func (r *Repository) RelayEndpointCredentials(tx *gorm.DB, tunnelID, nodeID int64, role int, create bool) (*relaysecurity.Credentials, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	var row model.RelayEndpointAuth
	find := func() error {
		return db.Where("tunnel_id = ? AND node_id = ? AND chain_type = ?", tunnelID, nodeID, role).First(&row).Error
	}
	err := find()
	if errors.Is(err, gorm.ErrRecordNotFound) && create {
		var existing int64
		if e := db.Model(&model.ChainTunnel{}).Where("tunnel_id = ? AND node_id = ? AND chain_type = ?", tunnelID, nodeID, role).Count(&existing).Error; e != nil {
			return nil, e
		}
		if existing > 0 {
			var tunnel model.Tunnel
			if e := db.First(&tunnel, tunnelID).Error; e != nil {
				return nil, e
			}
			if tunnel.Type == 2 || tunnel.RelayAuthMode == relaysecurity.Required {
				return nil, errors.New("required relay endpoint credentials unavailable; refusing regeneration")
			}
		}

		c, e := relaysecurity.New()
		if e != nil {
			return nil, e
		}
		row = model.RelayEndpointAuth{TunnelID: tunnelID, NodeID: nodeID, ChainType: role, Version: c.Version, Username: c.Username, Password: c.Password}
		if e = db.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; e != nil {
			return nil, e
		}
		row = model.RelayEndpointAuth{}
		err = find()
	}
	if err != nil {
		return nil, errors.New("required relay endpoint credentials unavailable")
	}
	c := &relaysecurity.Credentials{Version: row.Version, Username: row.Username, Password: row.Password}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return c, nil
}

func (r *Repository) SetTunnelRelayAuthModeTx(tx *gorm.DB, tunnelID int64, mode string) error {
	if mode != relaysecurity.Required && mode != relaysecurity.Legacy {
		return errors.New("invalid relay authentication mode")
	}
	return tx.Model(&model.Tunnel{}).Where("id = ?", tunnelID).Update("relay_auth_mode", mode).Error
}

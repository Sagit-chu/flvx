package model

// RelayEndpointAuth is private desired state, never an API or backup DTO.
// Retained endpoint identities keep their credentials across topology edits.
type RelayEndpointAuth struct {
	ID        int64  `gorm:"primaryKey;autoIncrement" json:"-"`
	TunnelID  int64  `gorm:"not null;uniqueIndex:idx_relay_endpoint" json:"-"`
	NodeID    int64  `gorm:"not null;uniqueIndex:idx_relay_endpoint" json:"-"`
	ChainType int    `gorm:"not null;uniqueIndex:idx_relay_endpoint" json:"-"`
	Version   int    `gorm:"not null" json:"-"`
	Username  string `gorm:"type:text;not null" json:"-"`
	Password  string `gorm:"type:text;not null" json:"-"`
}

func (RelayEndpointAuth) TableName() string { return "relay_endpoint_auth" }

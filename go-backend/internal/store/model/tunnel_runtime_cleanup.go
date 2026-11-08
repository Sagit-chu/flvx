package model

// TunnelRuntimeCleanup retains ownership until an obsolete local runtime is
// acknowledged removed. It intentionally survives deletion of its tunnel.
type TunnelRuntimeCleanup struct {
	ID       int64  `gorm:"primaryKey;autoIncrement"`
	TunnelID int64  `gorm:"not null;uniqueIndex:idx_tunnel_cleanup"`
	NodeID   int64  `gorm:"not null;uniqueIndex:idx_tunnel_cleanup"`
	Kind     string `gorm:"type:varchar(16);not null;uniqueIndex:idx_tunnel_cleanup"`
	Port     int    `gorm:"not null;default:0;uniqueIndex:idx_tunnel_cleanup"`
}

func (TunnelRuntimeCleanup) TableName() string { return "tunnel_runtime_cleanup" }

package model

// FederationPendingRelease keeps rollback work after a failed remote release.
// It is independent of tunnels, which may never have committed or be deleted.
type FederationPendingRelease struct {
	ID            string `gorm:"primaryKey;size:64"`
	RemoteURL     string `gorm:"not null"`
	RemoteToken   string `gorm:"not null"`
	BindingID     string
	ReservationID string
	ResourceKey   string
	CreatedTime   int64
}

func (FederationPendingRelease) TableName() string { return "federation_pending_release" }

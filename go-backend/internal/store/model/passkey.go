package model

// Passkey stores a WebAuthn credential for exactly one panel user.
type Passkey struct {
	ID             string `gorm:"primaryKey;type:varchar(512)"`
	UserID         int64  `gorm:"column:user_id;not null;index"`
	Name           string `gorm:"type:varchar(100);not null"`
	CredentialJSON string `gorm:"column:credential_json;type:text;not null"`
	CreatedAt      int64  `gorm:"column:created_at;not null"`
	LastUsedAt     int64  `gorm:"column:last_used_at;not null;default:0"`
}

func (Passkey) TableName() string { return "passkey" }

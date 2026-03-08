package entity

import "time"

// DecryptionSession 记录提案计票所需的阈值解密会话信息。
type DecryptionSession struct {
	ID         string    `gorm:"primaryKey;size:64" json:"id"`
	ProposalID string    `gorm:"size:64;not null;uniqueIndex" json:"proposal_id"`
	Threshold  int       `gorm:"not null" json:"threshold"`
	Trustees   int       `gorm:"not null" json:"trustees"`
	PublicKey  []byte    `gorm:"type:blob;not null" json:"public_key"`
	SharesJSON string    `gorm:"type:text;not null" json:"shares_json"`
	CreatedAt  time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt  time.Time `gorm:"not null" json:"updated_at"`
}

package entity

import "time"

// Vote 代表一次匿名投票记录，Nullifier 全局唯一用于防重放。
type Vote struct {
	ID         string    `gorm:"primaryKey;size:64" json:"id"`
	ProposalID string    `gorm:"index;size:64;not null" json:"proposal_id"`
	Nullifier  string    `gorm:"uniqueIndex;size:128;not null" json:"nullifier"`
	Cipher     []byte    `gorm:"not null" json:"cipher"`
	CreatedAt  time.Time `gorm:"not null" json:"created_at"`
}

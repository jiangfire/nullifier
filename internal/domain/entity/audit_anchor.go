package entity

import "time"

// AuditAnchor 记录可导出的审计锚点信息。
type AuditAnchor struct {
	ID         string    `gorm:"primaryKey;size:64" json:"id"`
	ProposalID string    `gorm:"size:64;not null;index" json:"proposal_id"`
	MerkleRoot []byte    `gorm:"not null" json:"merkle_root"`
	AnchorHash string    `gorm:"size:128;not null;index" json:"anchor_hash"`
	CreatedAt  time.Time `gorm:"not null" json:"created_at"`
}

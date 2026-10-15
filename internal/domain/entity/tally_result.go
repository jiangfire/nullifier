package entity

import "time"

// TallyResult 记录提案计票发布结果。
type TallyResult struct {
	ID          string         `gorm:"primaryKey;size:64" json:"id"`
	ProposalID  string         `gorm:"size:64;not null;uniqueIndex" json:"proposal_id"`
	TotalVotes  int            `gorm:"not null" json:"total_votes"`
	TallyJSON   string         `gorm:"type:text;not null" json:"tally_json"`
	MerkleRoot  []byte         `gorm:"type:blob;not null" json:"merkle_root"`
	AnchorHash  string         `gorm:"size:128;not null" json:"anchor_hash"`
	PublishedAt time.Time      `gorm:"not null" json:"published_at"`
	CreatedAt   time.Time      `gorm:"not null" json:"created_at"`
	UpdatedAt   time.Time      `gorm:"not null" json:"updated_at"`
	Tally       map[string]int `gorm:"-" json:"tally"`
}

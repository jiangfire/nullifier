package entity

import "time"

// VoteParticipation 仅记录某用户（哈希标识）是否已参与某提案投票，不记录票内容。
type VoteParticipation struct {
	ID         string    `gorm:"primaryKey;size:64" json:"id"`
	ProposalID string    `gorm:"size:64;not null;uniqueIndex:idx_vote_participation" json:"proposal_id"`
	UserHash   string    `gorm:"size:128;not null;uniqueIndex:idx_vote_participation" json:"user_hash"`
	VotedAt    time.Time `gorm:"not null" json:"voted_at"`
	CreatedAt  time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt  time.Time `gorm:"not null" json:"updated_at"`
}

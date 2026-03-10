package entity

import "time"

// Credential 代表匿名投票凭证。
type Credential struct {
	ID               string    `gorm:"primaryKey;size:64" json:"id"`
	ProposalID       string    `gorm:"size:64;not null;uniqueIndex:idx_credential_proposal_user" json:"proposal_id"`
	UserID           string    `gorm:"size:128;not null;uniqueIndex:idx_credential_proposal_user" json:"user_id"`
	UserPubKey       []byte    `gorm:"type:blob;not null" json:"user_pub_key"`
	Signature        []byte    `gorm:"type:blob;not null" json:"signature"`
	IssuerKeyVersion int       `gorm:"not null;default:1" json:"issuer_key_version"`
	IssuedAt         time.Time `gorm:"not null" json:"issued_at"`
}

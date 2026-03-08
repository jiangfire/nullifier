package entity

import "time"

// Credential 代表匿名投票凭证。
type Credential struct {
	ID               string    `gorm:"primaryKey;size:64" json:"id"`
	UserID           string    `gorm:"uniqueIndex;size:128;not null" json:"user_id"`
	UserPubKey       []byte    `gorm:"type:blob;not null" json:"user_pub_key"`
	Signature        []byte    `gorm:"type:blob;not null" json:"signature"`
	IssuerKeyVersion int       `gorm:"not null;default:1" json:"issuer_key_version"`
	IssuedAt         time.Time `gorm:"not null" json:"issued_at"`
}

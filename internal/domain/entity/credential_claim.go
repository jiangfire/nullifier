package entity

import "time"

// CredentialClaim 是凭证池里的一个待领取槽位。
//
// 池由管理端提前备好：每个槽位带一个随机 user_id（不绑定任何真人），
// 投票人匿名领取时写入自己的公钥并把槽位标记为已领取。
//
// 关键点：**服务端不记录「谁领了哪个槽位」** —— 领取请求里没有任何身份信息，
// 因此服务端与运营方都无法把凭证和投票人对应起来。
type CredentialClaim struct {
	ID         string     `gorm:"primaryKey;size:64" json:"id"`
	ProposalID string     `gorm:"size:64;not null;index" json:"proposal_id"`
	UserID     string     `gorm:"size:128;not null" json:"user_id"`
	UserPubKey []byte     `gorm:"not null" json:"user_pub_key"`
	Signature  []byte     `gorm:"not null" json:"signature"`
	ClaimedAt  *time.Time `gorm:"index" json:"claimed_at"`
	CreatedAt  time.Time  `gorm:"not null" json:"created_at"`
}

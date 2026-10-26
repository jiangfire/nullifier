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
	// ReservedAt 是这个槽位被**预留**的时刻（匿名领取的第一步）。
	//
	// 预留是**占位**：槽位在预留期间不会被别的领取者拿走，所以并发领取的人
	// 各拿各的槽位（没有预留的话，所有人都会拿到「第一个空闲槽位」，
	// 然后互相撞车）。它不是「已领取」—— 凭证还没签发，槽位最终归零。
	//
	// 预留会**过期**（见 usecase 里的 CredentialReservationTTL）：只预留不
	// 领取的客户端不能永久占住池子。
	ReservedAt *time.Time `gorm:"index" json:"reserved_at"`
	CreatedAt  time.Time  `gorm:"not null" json:"created_at"`
}

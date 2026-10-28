package entity

import "time"

// Proposal 代表一个投票提案。
type Proposal struct {
	ID          string    `gorm:"primaryKey;size:64" json:"id"`
	Title       string    `gorm:"size:256;not null" json:"title"`
	Description string    `gorm:"type:text" json:"description"`
	OptionsJSON string    `gorm:"type:text;default:'[]'" json:"-"`
	Options     []string  `gorm:"-" json:"options,omitempty"`
	Status      string    `gorm:"size:32;not null;default:open" json:"status"`
	StartsAt    time.Time `json:"starts_at"`
	EndsAt      time.Time `json:"ends_at"`
	CreatedAt   time.Time `gorm:"not null" json:"created_at"`
	// EligibilityScope 是本提案绑定的资格名单域（名字，可空）。
	//
	// 一台服务端可以并存多套名单（v0.4.0 起）：挑战签发与证明校验都按这个
	// 名字解析到对应名单的根、验证密钥与树深。为空表示未显式绑定 —— 取
	// 名为 default 的名单，只配置了一套名单时取它。
	EligibilityScope string `gorm:"size:64" json:"eligibility_scope,omitempty"`
}

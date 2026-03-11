package entity

import "time"

// Proposal 代表一个投票提案。
type Proposal struct {
	ID          string    `gorm:"primaryKey;size:64" json:"id"`
	Title       string    `gorm:"size:256;not null" json:"title"`
	Description string    `gorm:"type:text" json:"description"`
	OptionsJSON string    `gorm:"type:text;default:'[]'" json:"options,omitempty"`
	Options     []string  `gorm:"-" json:"options,omitempty"`
	Status      string    `gorm:"size:32;not null;default:open" json:"status"`
	StartsAt    time.Time `json:"starts_at"`
	EndsAt      time.Time `json:"ends_at"`
	CreatedAt   time.Time `gorm:"not null" json:"created_at"`
}

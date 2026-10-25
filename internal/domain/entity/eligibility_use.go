package entity

import "time"

// EligibilityUse 是一次「资格已用过」的记录 —— 「一人一票」的强制点。
//
// 它只存**去重键**（`QualificationID = MiMC(Leaf, ProposalID)`，十六进制），
// 不存叶子、不存资格秘密、不存成员 ID：去重键本身是 hiding 的，存它不等于
// 存身份。服务端因此只能知道「某个资格被用过」，不知道它属于谁。
//
// 唯一索引 `(proposal_id, qualification_id)` 就是一人一票的落地点：
// 同一成员在同一提案里第二次领取会撞唯一约束。
//
// ⚠️ 这张表**不能**与凭证、票据关联：一旦能把资格连到票，能力 G（分发不泄露
// 身份）就破了。去重只需要在领取时做一次。
type EligibilityUse struct {
	ID         string `gorm:"primaryKey;size:64" json:"id"`
	ProposalID string `gorm:"size:64;not null;uniqueIndex:idx_eligibility_use_proposal_qualification" json:"proposal_id"`
	// QualificationID 是去重键的十六进制编码（64 个字符）。
	QualificationID string    `gorm:"size:64;not null;uniqueIndex:idx_eligibility_use_proposal_qualification" json:"qualification_id"`
	UsedAt          time.Time `gorm:"not null" json:"used_at"`
}

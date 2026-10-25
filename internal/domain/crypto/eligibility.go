package crypto

// EligibilityStatement 是资格证明的公开输入。
//
// 全部用字符串表达：root / challenge / qualification_id 是十六进制的域元素，
// proposal_id 是提案 ID 原文。域元素之间的派生（提案域元素、会话绑定）
// 留给实现方按固定口径完成 —— 服务端与客户端各算一遍、算出两套是最容易
// 也最致命的错误。
type EligibilityStatement struct {
	// Root 是资格名单的 Merkle 树根（十六进制域元素）。
	Root string
	// ProposalID 是提案 ID 原文。
	ProposalID string
	// Challenge 是本次领取会话的挑战（十六进制域元素）。
	Challenge string
	// QualificationID 是去重键（十六进制域元素）。
	QualificationID string
}

// EligibilityService 是「一人一票」资格校验的服务端侧接口。
//
// 服务端只拿到名单的**根**，看不到名单本身：它能验证「这个人的资格在树里」，
// 却无法把资格对应到任何成员。
type EligibilityService interface {
	// Enabled 报告资格校验是否开启。
	//
	// 没开启时服务端**不强制**一人一票：任何人都能反复领取凭证。
	Enabled() bool
	// Root 返回名单树根（十六进制域元素）与树深。
	Root() (root string, depth int, err error)
	// NewChallenge 生成一个一次性的领取挑战（十六进制域元素）。
	NewChallenge() (string, error)
	// Verify 校验一份资格证明是否与公开输入一致。
	Verify(proof []byte, statement EligibilityStatement) error
}

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
//
// 一台服务端可以并存**多套**名单（v0.4.0 起）：每个名单是一个命名的作用域，
// 提案创建时绑定其中之一。服务端能验证「这个资格在这个名单的树里」，仍然
// 无法把资格对应到任何成员 —— 多名单不改变这条底线。
type EligibilityService interface {
	// Enabled 报告资格校验是否开启。
	//
	// 没开启时服务端**不强制**一人一票：任何人都能反复领取凭证。
	Enabled() bool
	// RosterFor 返回某个名单域的树根（十六进制域元素）与树深。
	//
	// scope 为空表示「未指定」：取默认名单 —— 名为 default 的名单；只配置了
	// 一个名单时取它。两者都不满足时返回错误（fail-closed）：多名单并存且
	// 没有默认名单时，提案必须显式绑定名单域。
	RosterFor(scope string) (root string, depth int, err error)
	// ScopeKnown 报告某个名单域是否存在。
	//
	// 用于提案创建时的前置校验（绑定了不存在的名单域当场 400，不留到领取时
	// 才失败）。空 scope 恒返回 true —— 它的解析在 RosterFor，且是否可用
	// 取决于届时服务器的名单配置。
	ScopeKnown(scope string) bool
	// NewChallenge 生成一个一次性的领取挑战（十六进制域元素）。
	NewChallenge() (string, error)
	// VerifyFor 校验一份资格证明是否与公开输入一致。
	//
	// root 用**该名单域**配置的那个，不接受调用方塞进来的：资格证明的意义
	// 就在于「服务端认的是自己那份名单」。
	VerifyFor(scope string, proof []byte, statement EligibilityStatement) error
}

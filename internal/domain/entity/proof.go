package entity

// NullifierByteSize 是 nullifier 的字节长度：BN254 标量域元素的规范编码。
const NullifierByteSize = 32

// Proof 封装零知识证明字节流。
type Proof struct {
	Data []byte `json:"data"`
}

// BallotStatement 是选票证明的公开输入声明。
//
// 公开输入由服务端根据请求字段、提案与计票会话自行拼装：客户端不提交
// 公开输入，因此一份 proof 只能用于它被生成的那一份密文，也没法跨提案
// 或跨 nullifier 复用。
type BallotStatement struct {
	// ProposalID 是提案 ID 原文。
	ProposalID string
	// Nullifier 是 32 字节的 nullifier 域元素。
	Nullifier []byte
	// ElectionPublicKey 是 32 字节压缩点形式的提案计票公钥。
	ElectionPublicKey []byte
	// Ciphertext 是 v3 选票密文。
	Ciphertext []byte
	// NumOptions 是提案的选项数。
	NumOptions int
}

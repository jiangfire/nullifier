package crypto

// NullifierGenerator 生成投票用的 nullifier。
//
// nullifier 是防重投的公开标识：它必须由投票人的秘密单向推导出来，且
// 编码规范唯一 —— 否则同一个 nullifier 会有多种字节表示，双重投票的
// 唯一索引就拦不住了。
type NullifierGenerator interface {
	// GenerateNullifier 生成一个规范编码的 nullifier（十六进制字符串）。
	GenerateNullifier() (string, error)
}

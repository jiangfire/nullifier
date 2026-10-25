package crypto

import (
	"context"
)

// IssuerPublicKey 是签发方的 RSA 公钥 —— 客户端盲签名只需要它。
//
// 公钥是公开值：盲化运算本来就是公开的（客户端要拿它把消息盲化）。
type IssuerPublicKey struct {
	// Version 是密钥版本。客户端必须用**同一版本**的公钥去盲化，服务端也要用
	// 同一版本的私钥去盲签 —— 版本对不上，解出来的签名验不过。
	Version int
	// Modulus 是模数 n（大端字节）。
	Modulus []byte
	// Exponent 是公开指数 e。
	Exponent int
	// Variant 是盲签名变体名（如 `SHA384PSSDeterministic`）。
	//
	// 客户端据此确认自己实现的变体与服务端一致 —— 变体不同，盲化结果就不同。
	Variant string
}

// CredentialIssuer 是签发方的**服务端侧**接口。
//
// 关键约定：**签发方在签名阶段只见盲化消息**。服务端只做 BlindSign ——
// Prepare / Blind / Finalize 三步全在客户端，客户端与服务端之间传的只有
// 盲化消息与盲签名。服务端任何时候都不持有被签消息的原文。
type CredentialIssuer interface {
	// BlindSign 对**盲化后**的消息做盲签名。
	//
	// keyVersion 必须是客户端盲化时用的那一版密钥：服务端要用同一把私钥做
	// 模幂运算，否则客户端解出来的签名是废的。
	BlindSign(ctx context.Context, keyVersion int, blindedMessage []byte) ([]byte, error)
	// IssuerPublicKey 返回当前签发方公钥，供客户端盲化消息。
	IssuerPublicKey(ctx context.Context) (*IssuerPublicKey, error)
	// VerifyCredential 用签发方的公钥校验凭证签名。
	//
	// 投票路径必须在落库前调用它：只比对「库里存的签名」与「请求里带的
	// 签名」是否逐字节相同，任何人都能拿一张合法凭证换个 user_id 重放。
	//
	// 返回值是**验签通过的那一版密钥** —— 凭证要记下它，密钥轮换后才知道
	// 该用哪把公钥验。
	VerifyCredential(userID string, userPubKey []byte, signature []byte) (int, error)
}

/**
 * 冻结的接口契约（v1）。
 *
 * **这些类型是「冻结」的** —— 字段名、字段的语义、端点路径从 v0.3.0 起只增不改。
 * 改它们是破坏性变更，必须走下一个 minor 版本。
 * 完整口径见 [`docs/api-contract-v1.md`](../../../docs/api-contract-v1.md)。
 *
 * 为什么要单独一个文件：门户这一侧只做 JSON 解析与转发，**最容易出错的不是数学
 * 而是字符串** —— 字段名一旦两边错开，表现往往不是报错，而是「服务端说字段缺失」
 * 或者「客户端发了个服务端不认的字段」。这里把线格式集中声明一次，
 * `tests/architecture/api_contract_test.go` 再拿它与 Go 侧的 struct tag 双向比对：
 * 改了一边不改另一边，CI 就红。
 *
 * ⚠️ **本文件与 Go 侧的 payload 结构体必须逐字段一致** —— 这是门禁的硬约束。
 */

/** 投票提交的请求体（六个字段全部必填）。 */
export interface VoteSubmitRequest {
  /** v3 选票密文的 base64 编码（65 字节）。 */
  encrypted_vote: string;
  /** Groth16 证明的 base64 编码。 */
  proof: string;
  /** 32 字节 nullifier 的规范 hex（64 位小写）。 */
  nullifier: string;
  /** 凭证槽位假名。 */
  user_id: string;
  /** 领取时生成的一次性公钥（base64）。 */
  user_pub_key: string;
  /** 凭证签名（base64）。 */
  credential_signature: string;
}

/**
 * 领取凭证的请求体。
 *
 * 三件材料要么都给、要么都不给 —— 只给一半服务端会拒（400）。
 * 都不给是合法的（服务端未开启资格校验时的形态）。
 */
export interface CredentialClaimRequest {
  /** 资格证明（base64 的 Groth16 证明）。 */
  eligibility_proof?: string;
  /** 去重键（十六进制的域元素）。 */
  qualification_id?: string;
  /** 服务端签发的领取挑战（十六进制的域元素）。 */
  challenge?: string;
}

/** 盲签名请求体（服务端只对盲化后的消息签名）。 */
export interface CredentialBlindSignRequest {
  /** 池槽位的假名。 */
  user_id: string;
  /** 客户端盲化后的消息（base64）。 */
  blinded_message: string;
  /** 客户端盲化时用的那一版签发方密钥。 */
  issuer_key_version: number;
}

/** attest 请求体（客户端交回解盲后的最终签名）。 */
export interface CredentialAttestRequest {
  /** 池槽位的假名。 */
  user_id: string;
  /** 领取者的一次性公钥（base64）。 */
  user_pub_key: string;
  /** 解盲后的最终签名（base64）。 */
  signature: string;
}

/** 预留槽位的响应。 */
export interface CredentialReservation {
  /** 提案 ID。 */
  proposal_id: string;
  /** 池槽位的假名。 */
  user_id: string;
  /** 签发方密钥版本。 */
  issuer_key_version: number;
  /** 盲签名变体的**线路名**（不是 Go 的枚举标识符）。 */
  variant: string;
  /** 签发方 RSA 模数（base64）。 */
  modulus: string;
  /** 签发方 RSA 指数。 */
  exponent: number;
}

/** 资格挑战的响应。 */
export interface EligibilityChallenge {
  /** 提案 ID。 */
  proposal_id: string;
  /** 资格名单的 Merkle 树根（十六进制的域元素）。 */
  root: string;
  /** 树深。 */
  depth: number;
  /** 本次领取挑战（十六进制的域元素）。 */
  challenge: string;
}

/**
 * 冻结的端点路径。
 *
 * 路径里一律带 `:proposal_id` —— 提案 ID 只有**一个**来源（路径），
 * 请求体里不再放第二份。改路径是破坏性变更。
 */
export const apiPaths = {
  /** 领取挑战。 */
  eligibilityChallenge: (proposalId: string): string =>
    `/api/v1/proposals/${encodeURIComponent(proposalId)}/credentials/eligibility-challenge`,
  /** 预留凭证槽位。 */
  credentialClaim: (proposalId: string): string =>
    `/api/v1/proposals/${encodeURIComponent(proposalId)}/credentials/claim`,
  /** 盲签名。 */
  credentialBlindSign: (proposalId: string): string =>
    `/api/v1/proposals/${encodeURIComponent(proposalId)}/credentials/blind-sign`,
  /** 交回解盲后的签名。 */
  credentialAttest: (proposalId: string): string =>
    `/api/v1/proposals/${encodeURIComponent(proposalId)}/credentials/attest`,
  /** 提交选票。 */
  voteSubmit: (proposalId: string): string =>
    `/api/v1/proposals/${encodeURIComponent(proposalId)}/votes`,
} as const;

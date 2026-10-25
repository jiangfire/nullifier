package usecase

import "errors"

var (
	ErrInvalidRequest          = errors.New("invalid request")
	ErrDoubleVoting            = errors.New("double voting detected")
	ErrCredentialAlreadyIssued = errors.New("credential already issued")
	ErrUserAlreadyVoted        = errors.New("user already voted")
	ErrInvalidCredential       = errors.New("invalid credential")
	ErrUnauthorized            = errors.New("unauthorized")
	ErrSessionAlreadyExists    = errors.New("decryption session already exists")
	ErrResultAlreadyPublished  = errors.New("tally result already published")
	// ErrCredentialPoolEmpty 表示凭证池已领完 —— 调用方应返回 4xx 而不是 5xx。
	ErrCredentialPoolEmpty = errors.New("credential pool is empty")
	// ErrEligibilityRequired 表示服务端要求资格证明，但客户端没带。
	ErrEligibilityRequired = errors.New("eligibility proof is required")
	// ErrInvalidEligibilityProof 表示资格证明没通过验证（伪造、名单不对、
	// 提案不对或挑战不对）。
	ErrInvalidEligibilityProof = errors.New("invalid eligibility proof")
	// ErrEligibilityAlreadyUsed 表示这份资格已经领过票 —— 一人一票的强制点。
	ErrEligibilityAlreadyUsed = errors.New("eligibility already used")
	// ErrEligibilityNotEnabled 表示服务端没有开启资格校验。
	ErrEligibilityNotEnabled = errors.New("eligibility verification is not enabled")
)

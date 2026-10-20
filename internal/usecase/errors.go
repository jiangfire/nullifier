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
)

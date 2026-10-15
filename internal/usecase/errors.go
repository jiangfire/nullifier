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
)

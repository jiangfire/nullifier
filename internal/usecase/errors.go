package usecase

import "errors"

var (
	ErrInvalidRequest          = errors.New("invalid request")
	ErrDoubleVoting            = errors.New("double voting detected")
	ErrCredentialAlreadyIssued = errors.New("credential already issued")
	ErrUserAlreadyVoted        = errors.New("user already voted")
	ErrInvalidCredential       = errors.New("invalid credential")
)

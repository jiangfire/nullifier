package repository

import "errors"

var (
	// ErrNotFound 表示仓储未找到目标对象。
	ErrNotFound = errors.New("not found")
	// ErrDuplicateVote 表示命中票据唯一约束（nullifier 重复）。
	ErrDuplicateVote = errors.New("duplicate vote")
	// ErrDuplicateParticipation 表示用户在同一提案下已存在参与记录。
	ErrDuplicateParticipation = errors.New("duplicate participation")
)

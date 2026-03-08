package repository

import "errors"

var (
	// ErrNotFound 表示仓储未找到目标对象。
	ErrNotFound = errors.New("not found")
)

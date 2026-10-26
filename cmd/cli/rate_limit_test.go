package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestHTTPError_MessageForRateLimited 断言限流错误给出「等一等就好」的提示。
//
// 限流是唯一一种重试就有用的失败 —— 报成「服务端出错」会让人以为出了故障。
func TestHTTPError_MessageForRateLimited(t *testing.T) {
	t.Parallel()

	err := &httpError{
		Status:  429,
		Code:    "RATE_LIMITED",
		Message: "rate limit exceeded",
	}
	require.Contains(t, err.Error(), "稍等再试")
	require.Contains(t, err.Error(), "RATE_LIMITED")
}

// TestHTTPError_MessageForOtherCodes 钉住其它错误码的格式不变。
func TestHTTPError_MessageForOtherCodes(t *testing.T) {
	t.Parallel()

	err := &httpError{
		Status:  409,
		Code:    "CREDENTIAL_EXISTS",
		Message: "credential already issued",
	}
	require.Equal(t, "credential already issued (CREDENTIAL_EXISTS)", err.Error())

	bare := &httpError{Status: 500, Message: "boom"}
	require.Equal(t, "boom", bare.Error())
}

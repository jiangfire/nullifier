package middleware

import (
	"errors"
	"net/http"

	"anonvote/internal/domain/repository"
	"anonvote/internal/usecase"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func ErrorHandler(logger *zap.Logger) gin.HandlerFunc {
	if logger == nil {
		logger = zap.NewNop()
	}

	return func(c *gin.Context) {
		c.Next()

		if len(c.Errors) == 0 {
			return
		}
		if c.Writer.Written() {
			return
		}

		err := c.Errors.Last().Err
		logger.Error("request failed", zap.Error(err))

		status, code := MapError(err)
		message := publicErrorMessage(err, code)
		c.JSON(status, gin.H{
			"error": message,
			"code":  code,
		})
	}
}

// RateLimitedCode 是限流触发的机器可读错误码。
//
// 它**不在** MapError 里：那张表映射的是领域错误（usecase.Err*），
// 而限流是传输层的保护措施 —— 它由限流中间件自己写 429 响应。
// 但文案仍然走 publicErrorMessage，客户端看到的是同一套错误体。
const RateLimitedCode = "RATE_LIMITED"

// MapError 把领域错误映射成 HTTP 状态码与机器可读的错误码。
//
// 它是**唯一**的错误码口径：响应体里的 code、指标里的 code 标签都出自这里。
// 导出它是为了让 handler 能在记录指标时拿到同一个码 —— 两处各写一份映射，
// 早晚会漂移。
func MapError(err error) (int, string) {
	switch {
	case errors.Is(err, usecase.ErrInvalidRequest):
		return http.StatusBadRequest, "INVALID_REQUEST"
	case errors.Is(err, usecase.ErrDoubleVoting):
		return http.StatusConflict, "DOUBLE_VOTING"
	case errors.Is(err, usecase.ErrCredentialAlreadyIssued):
		return http.StatusConflict, "CREDENTIAL_EXISTS"
	case errors.Is(err, usecase.ErrUserAlreadyVoted):
		return http.StatusConflict, "USER_ALREADY_VOTED"
	case errors.Is(err, usecase.ErrInvalidCredential):
		return http.StatusUnauthorized, "INVALID_CREDENTIAL"
	case errors.Is(err, usecase.ErrSessionAlreadyExists):
		return http.StatusConflict, "DECRYPTION_SESSION_EXISTS"
	case errors.Is(err, usecase.ErrResultAlreadyPublished):
		return http.StatusConflict, "RESULT_ALREADY_PUBLISHED"
	case errors.Is(err, usecase.ErrUnauthorized):
		return http.StatusUnauthorized, "UNAUTHORIZED"
	case errors.Is(err, usecase.ErrCredentialPoolEmpty):
		// 凭证池领完是运营状态问题，不是服务端故障 —— 4xx，别进 500。
		return http.StatusConflict, "CREDENTIAL_POOL_EMPTY"
	case errors.Is(err, usecase.ErrEligibilityRequired):
		// 客户端该带资格材料却没带 —— 请求不完整。
		return http.StatusBadRequest, "ELIGIBILITY_REQUIRED"
	case errors.Is(err, usecase.ErrInvalidEligibilityProof):
		// 证明没验过：伪造、名单不对、提案不对或挑战不对。
		return http.StatusForbidden, "INVALID_ELIGIBILITY_PROOF"
	case errors.Is(err, usecase.ErrEligibilityAlreadyUsed):
		// 一人一票的强制点：这份资格已经领过票了。
		return http.StatusConflict, "ELIGIBILITY_ALREADY_USED"
	case errors.Is(err, usecase.ErrEligibilityNotEnabled):
		// 服务端没有开启资格校验 —— 部署形态问题，不是客户端的错。
		return http.StatusConflict, "ELIGIBILITY_NOT_ENABLED"
	case errors.Is(err, repository.ErrNotFound):
		return http.StatusNotFound, "NOT_FOUND"
	default:
		return http.StatusInternalServerError, "INTERNAL_ERROR"
	}
}

func publicErrorMessage(err error, code string) string {
	switch code {
	case "INVALID_REQUEST":
		if err == nil {
			return "invalid request"
		}
		return err.Error()
	case "DOUBLE_VOTING":
		return "double voting"
	case "CREDENTIAL_EXISTS":
		return "credential already issued"
	case "USER_ALREADY_VOTED":
		return "user already voted"
	case "INVALID_CREDENTIAL":
		return "invalid credential"
	case "DECRYPTION_SESSION_EXISTS":
		return "decryption session already exists"
	case "RESULT_ALREADY_PUBLISHED":
		return "tally result already published"
	case "UNAUTHORIZED":
		return "unauthorized"
	case "CREDENTIAL_POOL_EMPTY":
		// 凭证池领完是运营状态问题，不是服务端故障 —— 别报成 internal error。
		return "credential pool is empty"
	case "ELIGIBILITY_REQUIRED":
		return "eligibility proof is required"
	case "INVALID_ELIGIBILITY_PROOF":
		return "invalid eligibility proof"
	case "ELIGIBILITY_ALREADY_USED":
		return "this eligibility has already been used"
	case "ELIGIBILITY_NOT_ENABLED":
		return "eligibility verification is not enabled on this server"
	case RateLimitedCode:
		return "rate limit exceeded"
	case "NOT_FOUND":
		return "resource not found"
	default:
		return "internal server error"
	}
}

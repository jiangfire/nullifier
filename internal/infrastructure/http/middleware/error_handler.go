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

		status, code := mapError(err)
		message := publicErrorMessage(code)
		c.JSON(status, gin.H{
			"error": message,
			"code":  code,
		})
	}
}

func mapError(err error) (int, string) {
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
	case errors.Is(err, usecase.ErrUnauthorized):
		return http.StatusUnauthorized, "UNAUTHORIZED"
	case errors.Is(err, repository.ErrNotFound):
		return http.StatusNotFound, "NOT_FOUND"
	default:
		return http.StatusInternalServerError, "INTERNAL_ERROR"
	}
}

func publicErrorMessage(code string) string {
	switch code {
	case "INVALID_REQUEST":
		return "invalid request"
	case "DOUBLE_VOTING":
		return "double voting"
	case "CREDENTIAL_EXISTS":
		return "credential already issued"
	case "USER_ALREADY_VOTED":
		return "user already voted"
	case "INVALID_CREDENTIAL":
		return "invalid credential"
	case "UNAUTHORIZED":
		return "unauthorized"
	case "NOT_FOUND":
		return "resource not found"
	default:
		return "internal server error"
	}
}

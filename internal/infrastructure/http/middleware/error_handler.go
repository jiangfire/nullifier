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
		c.JSON(status, gin.H{
			"error": err.Error(),
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
	case errors.Is(err, repository.ErrNotFound):
		return http.StatusNotFound, "NOT_FOUND"
	default:
		return http.StatusInternalServerError, "INTERNAL_ERROR"
	}
}

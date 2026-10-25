package middleware

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"anonvote/internal/domain/repository"
	"anonvote/internal/usecase"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestMapError(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{name: "invalid_request", err: usecase.ErrInvalidRequest, status: http.StatusBadRequest, code: "INVALID_REQUEST"},
		{name: "double_voting", err: usecase.ErrDoubleVoting, status: http.StatusConflict, code: "DOUBLE_VOTING"},
		{name: "credential_exists", err: usecase.ErrCredentialAlreadyIssued, status: http.StatusConflict, code: "CREDENTIAL_EXISTS"},
		{name: "user_voted", err: usecase.ErrUserAlreadyVoted, status: http.StatusConflict, code: "USER_ALREADY_VOTED"},
		{name: "invalid_credential", err: usecase.ErrInvalidCredential, status: http.StatusUnauthorized, code: "INVALID_CREDENTIAL"},
		{name: "session_exists", err: usecase.ErrSessionAlreadyExists, status: http.StatusConflict, code: "DECRYPTION_SESSION_EXISTS"},
		{name: "result_published", err: usecase.ErrResultAlreadyPublished, status: http.StatusConflict, code: "RESULT_ALREADY_PUBLISHED"},
		{name: "unauthorized", err: usecase.ErrUnauthorized, status: http.StatusUnauthorized, code: "UNAUTHORIZED"},
		{name: "credential_pool_empty", err: usecase.ErrCredentialPoolEmpty, status: http.StatusConflict, code: "CREDENTIAL_POOL_EMPTY"},
		{name: "eligibility_required", err: usecase.ErrEligibilityRequired, status: http.StatusBadRequest, code: "ELIGIBILITY_REQUIRED"},
		{name: "invalid_eligibility_proof", err: usecase.ErrInvalidEligibilityProof, status: http.StatusForbidden, code: "INVALID_ELIGIBILITY_PROOF"},
		{name: "eligibility_already_used", err: usecase.ErrEligibilityAlreadyUsed, status: http.StatusConflict, code: "ELIGIBILITY_ALREADY_USED"},
		{name: "eligibility_not_enabled", err: usecase.ErrEligibilityNotEnabled, status: http.StatusConflict, code: "ELIGIBILITY_NOT_ENABLED"},
		{name: "not_found", err: repository.ErrNotFound, status: http.StatusNotFound, code: "NOT_FOUND"},
		{name: "unknown", err: errors.New("x"), status: http.StatusInternalServerError, code: "INTERNAL_ERROR"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, code := mapError(tc.err)
			require.Equal(t, tc.status, status)
			require.Equal(t, tc.code, code)
		})
	}
}

func TestPublicErrorMessage(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code string
		want string
	}{
		{name: "invalid_request", err: usecase.ErrInvalidRequest, code: "INVALID_REQUEST", want: "invalid request"},
		{name: "session_exists", err: usecase.ErrSessionAlreadyExists, code: "DECRYPTION_SESSION_EXISTS", want: "decryption session already exists"},
		{name: "result_published", err: usecase.ErrResultAlreadyPublished, code: "RESULT_ALREADY_PUBLISHED", want: "tally result already published"},
		{name: "unauthorized", err: usecase.ErrUnauthorized, code: "UNAUTHORIZED", want: "unauthorized"},
		{name: "credential_pool_empty", err: usecase.ErrCredentialPoolEmpty, code: "CREDENTIAL_POOL_EMPTY", want: "credential pool is empty"},
		{name: "eligibility_required", err: usecase.ErrEligibilityRequired, code: "ELIGIBILITY_REQUIRED", want: "eligibility proof is required"},
		{name: "invalid_eligibility_proof", err: usecase.ErrInvalidEligibilityProof, code: "INVALID_ELIGIBILITY_PROOF", want: "invalid eligibility proof"},
		{name: "eligibility_already_used", err: usecase.ErrEligibilityAlreadyUsed, code: "ELIGIBILITY_ALREADY_USED", want: "this eligibility has already been used"},
		{name: "eligibility_not_enabled", err: usecase.ErrEligibilityNotEnabled, code: "ELIGIBILITY_NOT_ENABLED", want: "eligibility verification is not enabled on this server"},
		{name: "unknown", err: errors.New("sql: connection refused"), code: "INTERNAL_ERROR", want: "internal server error"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, publicErrorMessage(tc.err, tc.code))
		})
	}
}

func TestErrorHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(ErrorHandler(nil))
	engine.GET("/bad", func(c *gin.Context) {
		c.Error(usecase.ErrInvalidRequest)
	})
	engine.GET("/boom", func(c *gin.Context) {
		c.Error(errors.New("sql: connection refused"))
	})
	engine.GET("/ok", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/bad", nil)
	engine.ServeHTTP(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "INVALID_REQUEST", body["code"])
	require.Equal(t, "invalid request", body["error"])

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/bad", nil)
	engine = gin.New()
	engine.Use(ErrorHandler(nil))
	engine.GET("/bad", func(c *gin.Context) {
		c.Error(errors.Join(usecase.ErrInvalidRequest, errors.New("starts_at must be in the future")))
	})
	engine.ServeHTTP(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	body = map[string]any{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "INVALID_REQUEST", body["code"])
	require.Contains(t, body["error"], "starts_at must be in the future")

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/boom", nil)
	engine = gin.New()
	engine.Use(ErrorHandler(nil))
	engine.GET("/boom", func(c *gin.Context) {
		c.Error(errors.New("sql: connection refused"))
	})
	engine.ServeHTTP(rec, req)
	require.Equal(t, http.StatusInternalServerError, rec.Code)
	body = map[string]any{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "INTERNAL_ERROR", body["code"])
	require.Equal(t, "internal server error", body["error"])

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/ok", nil)
	engine = gin.New()
	engine.Use(ErrorHandler(nil))
	engine.GET("/ok", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	engine.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
}

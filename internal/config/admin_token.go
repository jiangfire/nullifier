package config

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
)

const generatedAdminTokenBytes = 32

// EnsureAdminBearerToken makes sure the server always has an admin token.
// When the token is missing from config/env, it generates an ephemeral token for the current process.
// Tiered tokens (server.admin_tokens) count as configured: the admin tier then
// comes from admin_tokens.admin, and generating an ephemeral token here would
// override it (see http.adminTokens) and leak the live token into the log.
func EnsureAdminBearerToken(cfg *Config) (bool, error) {
	if cfg == nil {
		return false, fmt.Errorf("config is nil")
	}

	if token := strings.TrimSpace(cfg.AdminBearerToken); token != "" {
		cfg.AdminBearerToken = token
		return false, nil
	}

	// 分级令牌任一档非空 = 已显式配置，不再生成。刻意只配 operator/auditor
	// 的分权部署也由此得到尊重：admin 档为空时 RequireAdminRole 对空候选集
	// fail-closed（admin 路由对所有人 401），不会退化成「无鉴权」。
	if cfg.AdminTokens != (AdminTokensConfig{}) {
		return false, nil
	}

	token, err := generateAdminBearerToken(generatedAdminTokenBytes)
	if err != nil {
		return false, fmt.Errorf("generate admin bearer token: %w", err)
	}

	cfg.AdminBearerToken = token
	return true, nil
}

func generateAdminBearerToken(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

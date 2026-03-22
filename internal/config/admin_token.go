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
func EnsureAdminBearerToken(cfg *Config) (bool, error) {
	if cfg == nil {
		return false, fmt.Errorf("config is nil")
	}

	if token := strings.TrimSpace(cfg.AdminBearerToken); token != "" {
		cfg.AdminBearerToken = token
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

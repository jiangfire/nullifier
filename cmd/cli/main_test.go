package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCLI_CreateProposal_SendsPlainDescriptionAndOptions(t *testing.T) {
	t.Parallel()

	type createProposalRequest struct {
		Title       string   `json:"title"`
		Description string   `json:"description"`
		Options     []string `json:"options"`
		StartsAt    string   `json:"starts_at"`
		EndsAt      string   `json:"ends_at"`
	}

	requests := make(chan createProposalRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/api/v1/proposals", r.URL.Path)
		require.Equal(t, "Bearer secret-token", r.Header.Get("Authorization"))

		var req createProposalRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		requests <- req

		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"id":          "proposal-1",
			"title":       req.Title,
			"description": req.Description,
			"options":     req.Options,
			"status":      "open",
			"starts_at":   req.StartsAt,
			"ends_at":     req.EndsAt,
			"created_at":  time.Now().UTC().Format(time.RFC3339),
		}))
	}))
	defer server.Close()

	c := &cli{
		baseURL:    server.URL,
		adminToken: "secret-token",
		httpClient: server.Client(),
	}

	output := captureStdout(t, func() {
		err := c.cmdCreateProposal([]string{
			"-title", "Board Election",
			"-description", "Vote for one candidate",
			"-options", "candidate-A,candidate-B",
			"-start", "2026-03-18T10:00:00Z",
			"-end", "2026-03-18T12:00:00Z",
		})
		require.NoError(t, err)
	})

	req := <-requests
	require.Equal(t, "Board Election", req.Title)
	require.Equal(t, "Vote for one candidate", req.Description)
	require.Equal(t, []string{"candidate-A", "candidate-B"}, req.Options)
	require.Contains(t, output, `"id": "proposal-1"`)
	require.NotContains(t, req.Description, `"summary"`)
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	original := os.Stdout
	reader, writer, err := os.Pipe()
	require.NoError(t, err)

	os.Stdout = writer
	defer func() {
		os.Stdout = original
	}()

	fn()

	require.NoError(t, writer.Close())
	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	return strings.TrimSpace(string(data))
}

package httpapi

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"anonvote/internal/config"
	"anonvote/internal/domain/entity"
	cryptoinfra "anonvote/internal/infrastructure/crypto"
	"anonvote/internal/infrastructure/persistence"
	"anonvote/internal/usecase"

	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark/backend/groth16"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/frontend/cs/r1cs"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type serverTestCircuit struct {
	X frontend.Variable
	Y frontend.Variable `gnark:",public"`
}

func (c *serverTestCircuit) Define(api frontend.API) error {
	x3 := api.Mul(c.X, c.X, c.X)
	api.AssertIsEqual(c.Y, api.Add(x3, c.X, 5))
	return nil
}

func TestServer_DefaultAndRunError(t *testing.T) {
	handler := NewHandler(nil, nil, nil, nil, nil)
	server := NewServer(nil, handler, nil)
	require.NotNil(t, server.Engine())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	server.Engine().ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	badServer := NewServer(&config.Config{
		ServerAddress: "127.0.0.1:-1",
		GinMode:       gin.TestMode,
	}, handler, zap.NewNop())
	err := badServer.Run()
	require.Error(t, err)
}

func TestHandler_WithNilUsecases(t *testing.T) {
	engine := NewServer(&config.Config{
		ServerAddress:    ":0",
		GinMode:          gin.TestMode,
		AdminBearerToken: "secret",
	}, NewHandlerWithTally(nil, nil, nil, nil, nil, zap.NewNop()), zap.NewNop()).Engine()

	cases := []struct {
		method string
		path   string
		body   any
	}{
		{method: http.MethodPost, path: "/api/v1/votes", body: map[string]any{}},
		{method: http.MethodPost, path: "/api/v1/proposals/p1/vote-status", body: map[string]any{}},
		{method: http.MethodGet, path: "/api/v1/proposals/open"},
		{method: http.MethodGet, path: "/api/v1/proposals"},
		{method: http.MethodPost, path: "/api/v1/proposals/p1/ballot-draft", body: map[string]any{}},
		{method: http.MethodGet, path: "/api/v1/proposals/p1/results"},
		{method: http.MethodGet, path: "/api/v1/proposals/p1/audit-anchors"},
	}

	for _, tc := range cases {
		rec := httptest.NewRecorder()
		req := jsonRequest(t, tc.method, tc.path, tc.body)
		engine.ServeHTTP(rec, req)
		require.Equal(t, http.StatusInternalServerError, rec.Code)
	}

	for _, tc := range []struct {
		method string
		path   string
		body   any
	}{
		{method: http.MethodPost, path: "/api/v1/credentials", body: map[string]any{}},
		{method: http.MethodPost, path: "/api/v1/proposals", body: map[string]any{}},
		{method: http.MethodPost, path: "/api/v1/proposals/p1/decryption/setup", body: map[string]any{}},
		{method: http.MethodPost, path: "/api/v1/proposals/p1/results/publish", body: map[string]any{}},
		{method: http.MethodGet, path: "/api/v1/proposals/p1/credentials"},
	} {
		rec := httptest.NewRecorder()
		req := jsonRequest(t, tc.method, tc.path, tc.body)
		req.Header.Set("Authorization", "Bearer secret")
		engine.ServeHTTP(rec, req)
		require.Equal(t, http.StatusInternalServerError, rec.Code)
	}
}

func TestHandler_InvalidPayload(t *testing.T) {
	handler := NewHandlerWithTally(
		usecase.NewVotingUseCase(nil, nil, nil, nil, nil, nil, nil, zap.NewNop()),
		usecase.NewRegisterUseCase(nil, nil, nil, zap.NewNop()),
		usecase.NewProposalUseCase(nil, zap.NewNop()),
		usecase.NewParticipationUseCase(nil, nil, zap.NewNop()),
		usecase.NewTallyUseCase(nil, nil, nil, nil, nil, nil, nil, zap.NewNop()),
		zap.NewNop(),
	)
	engine := NewServer(&config.Config{
		ServerAddress:    ":0",
		GinMode:          gin.TestMode,
		AdminBearerToken: "secret",
	}, handler, zap.NewNop()).Engine()

	cases := []struct {
		method string
		path   string
		body   any
	}{
		{
			method: http.MethodPost,
			path:   "/api/v1/votes",
			body: map[string]any{
				"proposal_id":          "p1",
				"encrypted_vote":       "%%%invalid-base64%%%",
				"proof":                base64.StdEncoding.EncodeToString([]byte("proof")),
				"nullifier":            "n1",
				"user_id":              "u1",
				"user_pub_key":         base64.StdEncoding.EncodeToString([]byte("pub")),
				"credential_signature": base64.StdEncoding.EncodeToString([]byte("sig")),
			},
		},
		{
			method: http.MethodPost,
			path:   "/api/v1/votes",
			body: map[string]any{
				"proposal_id":          "p1",
				"encrypted_vote":       base64.StdEncoding.EncodeToString([]byte("cipher")),
				"proof":                base64.StdEncoding.EncodeToString([]byte("proof")),
				"nullifier":            "n1",
				"public_witness":       "%%%invalid-base64%%%",
				"user_id":              "u1",
				"user_pub_key":         base64.StdEncoding.EncodeToString([]byte("pub")),
				"credential_signature": base64.StdEncoding.EncodeToString([]byte("sig")),
			},
		},
		{
			method: http.MethodPost,
			path:   "/api/v1/credentials",
			body: map[string]any{
				"proposal_id":  "p1",
				"user_id":      "u1",
				"user_pub_key": "%%%invalid-base64%%%",
			},
		},
		{
			method: http.MethodPost,
			path:   "/api/v1/proposals",
			body: map[string]any{
				"title":     "x",
				"options":   []string{"A", "B"},
				"starts_at": "bad-time",
				"ends_at":   "bad-time",
			},
		},
		{
			method: http.MethodPost,
			path:   "/api/v1/proposals/p1/vote-status",
			body: map[string]any{
				"user_id":              "u1",
				"user_pub_key":         "%%%invalid-base64%%%",
				"credential_signature": base64.StdEncoding.EncodeToString([]byte("sig")),
			},
		},
		{
			method: http.MethodPost,
			path:   "/api/v1/proposals/p1/decryption/setup",
			body: map[string]any{
				"threshold": 0,
				"trustees":  0,
			},
		},
		{
			method: http.MethodPost,
			path:   "/api/v1/proposals/p1/ballot-draft",
			body: map[string]any{
				"option": "",
			},
		},
		{
			method: http.MethodPost,
			path:   "/api/v1/proposals/p1/results/publish",
			body: map[string]any{
				"shares": []string{},
			},
		},
	}

	for _, tc := range cases {
		rec := httptest.NewRecorder()
		req := jsonRequest(t, tc.method, tc.path, tc.body)
		if tc.path == "/api/v1/credentials" || tc.path == "/api/v1/proposals" || tc.path == "/api/v1/proposals/p1/decryption/setup" || tc.path == "/api/v1/proposals/p1/results/publish" {
			req.Header.Set("Authorization", "Bearer secret")
		}
		engine.ServeHTTP(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code)
	}
}

func TestHandler_FailureBranches_AdditionalCoverage(t *testing.T) {
	handler := NewHandlerWithTally(
		usecase.NewVotingUseCase(nil, nil, nil, nil, nil, nil, nil, zap.NewNop()),
		usecase.NewRegisterUseCase(nil, nil, nil, zap.NewNop()),
		usecase.NewProposalUseCase(nil, zap.NewNop()),
		usecase.NewParticipationUseCase(nil, nil, zap.NewNop()),
		usecase.NewTallyUseCase(nil, nil, nil, nil, nil, nil, nil, zap.NewNop()),
		zap.NewNop(),
	)
	engine := NewServer(&config.Config{
		ServerAddress:    ":0",
		GinMode:          gin.TestMode,
		AdminBearerToken: "secret",
	}, handler, zap.NewNop()).Engine()

	malformedCases := []struct {
		method string
		path   string
		raw    string
	}{
		{method: http.MethodPost, path: "/api/v1/votes", raw: "{"},
		{method: http.MethodPost, path: "/api/v1/credentials", raw: "{"},
		{method: http.MethodPost, path: "/api/v1/proposals", raw: "{"},
		{method: http.MethodPost, path: "/api/v1/proposals/p1/vote-status", raw: "{"},
		{method: http.MethodPost, path: "/api/v1/proposals/p1/ballot-draft", raw: "{"},
		{method: http.MethodPost, path: "/api/v1/proposals/p1/decryption/setup", raw: "{"},
		{method: http.MethodPost, path: "/api/v1/proposals/p1/results/publish", raw: "{"},
	}
	for _, tc := range malformedCases {
		rec := httptest.NewRecorder()
		req := rawRequest(tc.method, tc.path, tc.raw)
		if tc.path == "/api/v1/credentials" || tc.path == "/api/v1/proposals" || tc.path == "/api/v1/proposals/p1/decryption/setup" || tc.path == "/api/v1/proposals/p1/results/publish" {
			req.Header.Set("Authorization", "Bearer secret")
		}
		engine.ServeHTTP(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code)
	}

	invalidDecodeCases := []struct {
		method string
		path   string
		body   any
	}{
		{
			method: http.MethodPost,
			path:   "/api/v1/votes",
			body: map[string]any{
				"proposal_id":          "p1",
				"encrypted_vote":       base64.StdEncoding.EncodeToString([]byte("cipher")),
				"proof":                "%%%invalid-base64%%%",
				"nullifier":            "n-proof",
				"user_id":              "u1",
				"user_pub_key":         base64.StdEncoding.EncodeToString([]byte("pub")),
				"credential_signature": base64.StdEncoding.EncodeToString([]byte("sig")),
			},
		},
		{
			method: http.MethodPost,
			path:   "/api/v1/votes",
			body: map[string]any{
				"proposal_id":          "p1",
				"encrypted_vote":       base64.StdEncoding.EncodeToString([]byte("cipher")),
				"proof":                base64.StdEncoding.EncodeToString([]byte("proof")),
				"nullifier":            "n-pub",
				"user_id":              "u1",
				"user_pub_key":         "%%%invalid-base64%%%",
				"credential_signature": base64.StdEncoding.EncodeToString([]byte("sig")),
			},
		},
		{
			method: http.MethodPost,
			path:   "/api/v1/votes",
			body: map[string]any{
				"proposal_id":          "p1",
				"encrypted_vote":       base64.StdEncoding.EncodeToString([]byte("cipher")),
				"proof":                base64.StdEncoding.EncodeToString([]byte("proof")),
				"nullifier":            "n-sig",
				"user_id":              "u1",
				"user_pub_key":         base64.StdEncoding.EncodeToString([]byte("pub")),
				"credential_signature": "%%%invalid-base64%%%",
			},
		},
		{
			method: http.MethodPost,
			path:   "/api/v1/proposals",
			body: map[string]any{
				"title":     "x",
				"options":   []string{"A", "B"},
				"starts_at": time.Now().UTC().Format(time.RFC3339),
				"ends_at":   "bad-time",
			},
		},
		{
			method: http.MethodPost,
			path:   "/api/v1/proposals/p1/vote-status",
			body: map[string]any{
				"user_id":              "u1",
				"user_pub_key":         "%%%invalid-base64%%%",
				"credential_signature": base64.StdEncoding.EncodeToString([]byte("sig")),
			},
		},
	}
	for _, tc := range invalidDecodeCases {
		rec := httptest.NewRecorder()
		req := jsonRequest(t, tc.method, tc.path, tc.body)
		if tc.path == "/api/v1/proposals" {
			req.Header.Set("Authorization", "Bearer secret")
		}
		engine.ServeHTTP(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code)
	}

	runtimeErrorCases := []struct {
		method string
		path   string
		body   any
	}{
		{method: http.MethodGet, path: "/api/v1/proposals/open"},
		{method: http.MethodGet, path: "/api/v1/proposals"},
		{method: http.MethodPost, path: "/api/v1/proposals/p1/vote-status", body: map[string]any{
			"user_id":              "u1",
			"user_pub_key":         base64.StdEncoding.EncodeToString([]byte("pub")),
			"credential_signature": base64.StdEncoding.EncodeToString([]byte("sig")),
		}},
		{method: http.MethodGet, path: "/api/v1/proposals/p1/results"},
		{method: http.MethodGet, path: "/api/v1/proposals/p1/audit-anchors"},
	}
	for _, tc := range runtimeErrorCases {
		rec := httptest.NewRecorder()
		req := jsonRequest(t, tc.method, tc.path, tc.body)
		engine.ServeHTTP(rec, req)
		require.Equal(t, http.StatusInternalServerError, rec.Code)
	}
}

func TestHandler_GetResult_NotFound(t *testing.T) {
	engine, _, _ := buildFullStackEngine(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/proposals/not-exist/results", nil)
	engine.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHandler_SuccessFlow(t *testing.T) {
	engine, _, db := buildFullStackEngine(t)
	ts := httptest.NewServer(engine)
	defer ts.Close()

	credentialReq := map[string]any{
		"proposal_id":  "",
		"user_id":      "u-1",
		"user_pub_key": base64.StdEncoding.EncodeToString([]byte("user-pub-key")),
	}
	var resp *http.Response
	proposalReq := map[string]any{
		"title":       "Board Election",
		"description": "Vote",
		"options":     []string{"candidate-A", "candidate-B"},
		"starts_at":   time.Now().UTC().Add(30 * time.Minute).Format(time.RFC3339),
		"ends_at":     time.Now().UTC().Add(90 * time.Minute).Format(time.RFC3339),
	}
	resp = doPostAdminJSON(t, ts.URL+"/api/v1/proposals", "secret", proposalReq)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var proposalResp struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&proposalResp))
	_ = resp.Body.Close()

	credentialReq["proposal_id"] = proposalResp.ID
	resp = doPostAdminJSON(t, ts.URL+"/api/v1/credentials", "secret", credentialReq)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var credentialResp struct {
		UserPubKey string `json:"user_pub_key"`
		Signature  string `json:"signature"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&credentialResp))
	_ = resp.Body.Close()

	statuses := doGetProposalCredentialStatuses(t, ts.URL, proposalResp.ID)
	require.Len(t, statuses, 1)
	require.Equal(t, "u-1", statuses[0].UserID)
	require.False(t, statuses[0].HasVoted)

	setupReq := map[string]any{
		"threshold": 2,
		"trustees":  3,
	}
	resp = doPostAdminJSON(t, ts.URL+"/api/v1/proposals/"+proposalResp.ID+"/decryption/setup", "secret", setupReq)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var setupResp struct {
		PublicKey string   `json:"public_key"`
		Shares    []string `json:"shares"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&setupResp))
	_ = resp.Body.Close()
	require.Len(t, setupResp.Shares, 3)

	require.NoError(t, db.Model(&entity.Proposal{}).
		Where("id = ?", proposalResp.ID).
		Updates(map[string]any{
			"starts_at": time.Now().UTC().Add(-5 * time.Minute),
			"ends_at":   time.Now().UTC().Add(30 * time.Minute),
		}).Error)

	draftReq := map[string]any{"option": "candidate-A"}
	resp = doPostJSON(t, ts.URL+"/api/v1/proposals/"+proposalResp.ID+"/ballot-draft", draftReq)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var draftResp struct {
		EncryptedVote string `json:"encrypted_vote"`
		Proof         string `json:"proof"`
		Nullifier     string `json:"nullifier"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&draftResp))
	_ = resp.Body.Close()
	require.NotEmpty(t, draftResp.EncryptedVote)
	require.NotEmpty(t, draftResp.Proof)
	require.NotEmpty(t, draftResp.Nullifier)

	var err error
	resp = doPostJSON(t, ts.URL+"/api/v1/proposals/"+proposalResp.ID+"/vote-status", map[string]any{
		"user_id":              "u-1",
		"user_pub_key":         credentialResp.UserPubKey,
		"credential_signature": credentialResp.Signature,
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

	voteReq := map[string]any{
		"proposal_id":          proposalResp.ID,
		"encrypted_vote":       draftResp.EncryptedVote,
		"proof":                draftResp.Proof,
		"nullifier":            draftResp.Nullifier,
		"user_id":              "u-1",
		"user_pub_key":         credentialResp.UserPubKey,
		"credential_signature": credentialResp.Signature,
	}
	resp = doPostJSON(t, ts.URL+"/api/v1/votes", voteReq)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	_ = resp.Body.Close()

	statuses = doGetProposalCredentialStatuses(t, ts.URL, proposalResp.ID)
	require.Len(t, statuses, 1)
	require.True(t, statuses[0].HasVoted)

	resp = doPostJSON(t, ts.URL+"/api/v1/proposals/"+proposalResp.ID+"/vote-status", map[string]any{
		"user_id":              "u-1",
		"user_pub_key":         credentialResp.UserPubKey,
		"credential_signature": credentialResp.Signature,
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

	resp, err = http.Get(ts.URL + "/api/v1/proposals/open")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

	resp, err = http.Get(ts.URL + "/api/v1/proposals")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var allProposals struct {
		Items []struct {
			ID     string   `json:"id"`
			Title  string   `json:"title"`
			Status string   `json:"status"`
			Opts   []string `json:"options"`
		} `json:"items"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&allProposals))
	_ = resp.Body.Close()
	require.NotEmpty(t, allProposals.Items, "GET /proposals must list proposals, not only open ones")
	found := false
	for _, item := range allProposals.Items {
		if item.ID == proposalResp.ID {
			found = true
			require.NotEmpty(t, item.Opts, "options must be serialized")
		}
	}
	require.True(t, found, "the created proposal must appear in GET /proposals")

	require.NoError(t, db.Model(&entity.Proposal{}).
		Where("id = ?", proposalResp.ID).
		Updates(map[string]any{
			"status":  "closed",
			"ends_at": time.Now().UTC().Add(-1 * time.Second),
		}).Error)

	publishReq := map[string]any{
		"shares": setupResp.Shares[:2],
	}
	resp = doPostAdminJSON(t, ts.URL+"/api/v1/proposals/"+proposalResp.ID+"/results/publish", "secret", publishReq)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	_ = resp.Body.Close()

	resp, err = http.Get(ts.URL + "/api/v1/proposals/" + proposalResp.ID + "/results")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

	resp, err = http.Get(ts.URL + "/api/v1/proposals/" + proposalResp.ID + "/audit-anchors")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()
}

type proposalCredentialStatus struct {
	CredentialID string `json:"credential_id"`
	UserID       string `json:"user_id"`
	HasVoted     bool   `json:"has_voted"`
}

func doGetProposalCredentialStatuses(
	t *testing.T,
	baseURL string,
	proposalID string,
) []proposalCredentialStatus {
	t.Helper()

	req, err := http.NewRequest(
		http.MethodGet,
		baseURL+"/api/v1/proposals/"+proposalID+"/credentials",
		nil,
	)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer secret")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body struct {
		Items []proposalCredentialStatus `json:"items"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	return body.Items
}

func buildFullStackEngine(t *testing.T) (*gin.Engine, *cryptoinfra.KyberElGamal, *gorm.DB) {
	t.Helper()

	db, err := persistence.NewDB(&persistence.DBConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	})
	require.NoError(t, err)
	require.NoError(t, persistence.AutoMigrate(db))

	voteRepo := persistence.NewVoteRepository(db)
	credentialRepo := persistence.NewCredentialRepository(db)
	proposalRepo := persistence.NewProposalRepository(db)
	participationRepo := persistence.NewParticipationRepository(db)
	sessionRepo := persistence.NewDecryptionSessionRepository(db)
	resultRepo := persistence.NewTallyResultRepository(db)
	anchorRepo := persistence.NewAuditAnchorRepository(db)

	proofVerifier, err := cryptoinfra.NewGnarkProofVerifier("")
	require.NoError(t, err)
	encrypter := cryptoinfra.NewKyberElGamal()
	issuer := cryptoinfra.NewKryptologyCredentialIssuerWithFile("", 0)
	merkleBuilder := cryptoinfra.NewGnarkMerkleRootBuilder()

	logger := zap.NewNop()
	votingUC := usecase.NewVotingUseCase(
		voteRepo,
		participationRepo,
		credentialRepo,
		proposalRepo,
		sessionRepo,
		proofVerifier,
		encrypter,
		logger,
	)
	registerUC := usecase.NewRegisterUseCase(credentialRepo, proposalRepo, issuer, logger)
	proposalUC := usecase.NewProposalUseCase(proposalRepo, logger)
	participationUC := usecase.NewParticipationUseCase(participationRepo, credentialRepo, logger)
	tallyUC := usecase.NewTallyUseCase(voteRepo, proposalRepo, sessionRepo, resultRepo, anchorRepo, encrypter, merkleBuilder, logger)

	handler := NewHandlerWithTally(votingUC, registerUC, proposalUC, participationUC, tallyUC, logger)
	server := NewServer(&config.Config{
		ServerAddress:    ":0",
		GinMode:          gin.TestMode,
		AdminBearerToken: "secret",
	}, handler, logger)
	return server.Engine(), encrypter, db
}

func buildProofBytes(t *testing.T, x int, y int) []byte {
	t.Helper()

	var circuit serverTestCircuit
	ccs, err := frontend.Compile(ecc.BN254, r1cs.NewBuilder, &circuit)
	require.NoError(t, err)
	pk, _, err := groth16.Setup(ccs)
	require.NoError(t, err)

	assignment := serverTestCircuit{X: x, Y: y}
	witness, err := frontend.NewWitness(&assignment, ecc.BN254)
	require.NoError(t, err)

	proof, err := groth16.Prove(ccs, pk, witness)
	require.NoError(t, err)

	var buf bytes.Buffer
	_, err = proof.WriteTo(&buf)
	require.NoError(t, err)
	return buf.Bytes()
}

func doPostJSON(t *testing.T, url string, body any) *http.Response {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	resp, err := http.Post(url, "application/json", bytes.NewReader(raw))
	require.NoError(t, err)
	return resp
}

func doPostAdminJSON(t *testing.T, url string, token string, body any) *http.Response {
	t.Helper()

	raw, err := json.Marshal(body)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(raw))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func jsonRequest(t *testing.T, method string, path string, body any) *http.Request {
	t.Helper()
	if body == nil {
		return httptest.NewRequest(method, path, nil)
	}
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func rawRequest(method string, path string, raw string) *http.Request {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(raw))
	req.Header.Set("Content-Type", "application/json")
	return req
}

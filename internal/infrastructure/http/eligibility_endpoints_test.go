package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

// 这一组覆盖资格材料在 **HTTP 层**的解析分支。
//
// 成功路径与「一人一票」的性质在 tests/e2e/eligibility_test.go；这里补的是
// 「客户端发来不合法的东西」时服务端怎么答 —— 每一处都必须是 4xx，不能 500。

// 领取端点：资格材料必须三件齐备，给了一半要 400。
func TestReserveCredential_RejectsPartialEligibilityMaterial(t *testing.T) {
	engine, _, _ := buildFullStackEngineWithVerifier(t, nil)
	ts := httptest.NewServer(engine)
	defer ts.Close()

	proposalID := createProposalForClaim(t, ts)
	client := newBlindClient(t, ts)
	require.NoError(t, client.FillCredentialPool(proposalID, 1))

	cases := []struct {
		name string
		body map[string]any
	}{
		{
			name: "只有去重键",
			body: map[string]any{"qualification_id": "aa"},
		},
		{
			name: "只有证明",
			body: map[string]any{"eligibility_proof": "AA=="},
		},
		{
			name: "只有挑战",
			body: map[string]any{"challenge": "aa"},
		},
		{
			name: "证明不是 base64",
			body: map[string]any{
				"eligibility_proof": "%%%not-base64%%%",
				"qualification_id":  "aa",
				"challenge":         "bb",
			},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			resp := doPostJSON(
				t,
				ts.URL+"/api/v1/proposals/"+proposalID+"/credentials/claim",
				testCase.body,
			)
			defer func() { _ = resp.Body.Close() }()
			require.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})
	}
}

// 领取端点：空请求体在「未开启资格校验」时仍然合法（既有客户端的形态）。
func TestReserveCredential_AcceptsEmptyBodyWhenEligibilityDisabled(t *testing.T) {
	engine, _, _ := buildFullStackEngineWithVerifier(t, nil)
	ts := httptest.NewServer(engine)
	defer ts.Close()

	proposalID := createProposalForClaim(t, ts)
	client := newBlindClient(t, ts)
	require.NoError(t, client.FillCredentialPool(proposalID, 1))

	resp := doPostJSON(t, ts.URL+"/api/v1/proposals/"+proposalID+"/credentials/claim", map[string]any{})
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusCreated, resp.StatusCode)
}

// 挑战端点：服务端没开启资格校验时明确回答「没开启」，而不是假装发一个。
func TestIssueEligibilityChallenge_ReportsNotEnabled(t *testing.T) {
	engine, _, _ := buildFullStackEngineWithVerifier(t, nil)
	ts := httptest.NewServer(engine)
	defer ts.Close()

	proposalID := createProposalForClaim(t, ts)

	resp := doPostJSON(
		t,
		ts.URL+"/api/v1/proposals/"+proposalID+"/credentials/eligibility-challenge",
		map[string]any{},
	)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusConflict, resp.StatusCode)

	var failure struct {
		Code string `json:"code"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&failure))
	require.Equal(t, "ELIGIBILITY_NOT_ENABLED", failure.Code)
}

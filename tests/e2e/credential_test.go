package e2e

import (
	"net/http/httptest"
	"testing"

	"anonvote/tests/testutil"

	"github.com/stretchr/testify/require"
)

// issueCredential 给用户签发一张凭证（走真两段式盲签名）。
//
// 服务端只见盲化消息：客户端本地盲化 → 服务端盲签名 → 客户端解盲 → 服务端验签。
func issueCredential(
	t *testing.T,
	ts *httptest.Server,
	adminToken string,
	proposalID string,
	userID string,
	userPubKey []byte,
) *testutil.Credential {
	t.Helper()
	client := testutil.NewBlindClient(ts.URL)
	client.AdminToken = adminToken
	credential, err := client.IssueCredential(proposalID, userID, userPubKey)
	require.NoError(t, err)
	return credential
}

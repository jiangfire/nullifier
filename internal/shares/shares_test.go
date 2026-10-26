package shares

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_Formats(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	values, err := Load("s1, s2", "")
	require.NoError(t, err)
	require.Equal(t, []string{"s1", "s2"}, values)

	jsonPath := filepath.Join(dir, "shares.json")
	require.NoError(t, os.WriteFile(jsonPath, []byte(`["s1","s2"]`), 0o600))
	values, err = Load("", jsonPath)
	require.NoError(t, err)
	require.Equal(t, []string{"s1", "s2"}, values)

	// 会话对象：`cli decrypt-setup -out` 写出来的就是它。
	sessionPath := filepath.Join(dir, "session.json")
	require.NoError(t, os.WriteFile(sessionPath, []byte(
		`{"proposal_id":"p1","threshold":2,"trustees":3,"shares":["s1","s2","s3"]}`,
	), 0o600))
	values, err = Load("", sessionPath)
	require.NoError(t, err)
	require.Equal(t, []string{"s1", "s2", "s3"}, values)

	textPath := filepath.Join(dir, "shares.txt")
	require.NoError(t, os.WriteFile(textPath, []byte("s1 s2\ns3"), 0o600))
	values, err = Load("", textPath)
	require.NoError(t, err)
	require.Equal(t, []string{"s1", "s2", "s3"}, values)
}

func TestLoad_Errors(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	emptyPath := filepath.Join(dir, "empty.txt")
	require.NoError(t, os.WriteFile(emptyPath, []byte("  \n "), 0o600))
	_, err := Load("", emptyPath)
	require.Error(t, err)

	_, err = Load("", "")
	require.Error(t, err)

	_, err = Load("", filepath.Join(dir, "missing.txt"))
	require.Error(t, err)
}

func TestSplitCSV(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []string{"A", "B"}, SplitCSV("A, B"))
	assert.Equal(t, []string{"A"}, SplitCSV("A,, ,"))
	assert.Empty(t, SplitCSV("  ,  "))
}

func TestHash_IsStableAndDistinct(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Hash("share-1"), Hash("share-1"))
	assert.NotEqual(t, Hash("share-1"), Hash("share-2"))
	assert.Len(t, Hash("share-1"), 64)
}

func TestHashList(t *testing.T) {
	t.Parallel()

	list := HashList([]string{"a", "b"})
	require.Len(t, list, 2)
	assert.Equal(t, Hash("a"), list[0])
	assert.Equal(t, Hash("b"), list[1])
}

func TestParseRegistered(t *testing.T) {
	t.Parallel()

	parsed, err := ParseRegistered(`["` + Hash("s1") + `"]`)
	require.NoError(t, err)
	require.Len(t, parsed, 1)
	assert.Equal(t, Hash("s1"), parsed[0])

	_, err = ParseRegistered("")
	require.Error(t, err)

	_, err = ParseRegistered(`[]`)
	require.Error(t, err)

	_, err = ParseRegistered(`{bad-json`)
	require.Error(t, err)
}

func TestEnsureBelongToSession(t *testing.T) {
	t.Parallel()

	registered := HashList([]string{"share-1", "share-2", "share-3"})

	require.NoError(t, EnsureBelongToSession(registered, []string{"share-1", "share-2"}))

	// 别的会话的份额不能凑数。
	err := EnsureBelongToSession(registered, []string{"share-1", "other"})
	require.Error(t, err)

	// 同一份份额重复提交不能计入门限份数。
	err = EnsureBelongToSession(registered, []string{"share-1", "share-1"})
	require.Error(t, err)
}

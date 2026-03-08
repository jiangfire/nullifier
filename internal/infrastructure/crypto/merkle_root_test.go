package crypto

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGnarkMerkleRootBuilder_Deterministic(t *testing.T) {
	builder := NewGnarkMerkleRootBuilder()
	leaves := [][]byte{
		[]byte("leaf-a"),
		[]byte("leaf-b"),
		[]byte("leaf-c"),
	}

	root1, err := builder.BuildRoot(leaves)
	require.NoError(t, err)
	root2, err := builder.BuildRoot(leaves)
	require.NoError(t, err)

	require.Equal(t, root1, root2)
	require.NotEmpty(t, root1)
}

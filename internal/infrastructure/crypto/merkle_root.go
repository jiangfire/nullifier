package crypto

import (
	"crypto/sha256"
	"errors"

	"github.com/consensys/gnark-crypto/accumulator/merkletree"
)

// GnarkMerkleRootBuilder 使用 gnark-crypto 的 merkletree 构建根哈希。
type GnarkMerkleRootBuilder struct{}

func NewGnarkMerkleRootBuilder() *GnarkMerkleRootBuilder {
	return &GnarkMerkleRootBuilder{}
}

func (b *GnarkMerkleRootBuilder) BuildRoot(leaves [][]byte) ([]byte, error) {
	if len(leaves) == 0 {
		return nil, errors.New("leaves are empty")
	}

	tree := merkletree.New(sha256.New())
	for _, leaf := range leaves {
		if len(leaf) == 0 {
			return nil, errors.New("leaf is empty")
		}
		tree.Push(append([]byte(nil), leaf...))
	}

	root := tree.Root()
	if len(root) == 0 {
		return nil, errors.New("merkle root is empty")
	}
	return root, nil
}

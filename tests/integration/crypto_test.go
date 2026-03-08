package integration

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"anonvote/internal/domain/entity"
	cryptoinfra "anonvote/internal/infrastructure/crypto"

	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark/backend/groth16"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/frontend/cs/r1cs"
	"github.com/stretchr/testify/require"
)

type integrationCircuit struct {
	X frontend.Variable
	Y frontend.Variable `gnark:",public"`
}

func (c *integrationCircuit) Define(api frontend.API) error {
	x3 := api.Mul(c.X, c.X, c.X)
	api.AssertIsEqual(c.Y, api.Add(x3, c.X, 5))
	return nil
}

func TestCryptoPipeline_Integration(t *testing.T) {
	// gnark proving & verification
	var circuit integrationCircuit
	ccs, err := frontend.Compile(ecc.BN254, r1cs.NewBuilder, &circuit)
	require.NoError(t, err)

	pk, vk, err := groth16.Setup(ccs)
	require.NoError(t, err)

	assignment := integrationCircuit{X: 3, Y: 35}
	witness, err := frontend.NewWitness(&assignment, ecc.BN254)
	require.NoError(t, err)
	publicWitness, err := witness.Public()
	require.NoError(t, err)

	proof, err := groth16.Prove(ccs, pk, witness)
	require.NoError(t, err)

	var proofBuf bytes.Buffer
	_, err = proof.WriteTo(&proofBuf)
	require.NoError(t, err)
	publicWitnessBytes, err := publicWitness.MarshalBinary()
	require.NoError(t, err)

	var vkBuf bytes.Buffer
	_, err = vk.WriteTo(&vkBuf)
	require.NoError(t, err)

	vkPath := filepath.Join(t.TempDir(), "vk.bin")
	require.NoError(t, os.WriteFile(vkPath, vkBuf.Bytes(), 0o600))

	verifier, err := cryptoinfra.NewGnarkProofVerifier(vkPath)
	require.NoError(t, err)
	err = verifier.Verify(&entity.Proof{Data: proofBuf.Bytes()}, entity.PublicInputs{
		"public_witness": publicWitnessBytes,
	})
	require.NoError(t, err)

	// kyber encrypt & decrypt
	elgamal := cryptoinfra.NewKyberElGamal()
	priv, pub, err := elgamal.GenerateKeyPair()
	require.NoError(t, err)
	ciphertext, err := elgamal.Encrypt(pub, []byte("candidate-A"))
	require.NoError(t, err)
	plaintext, err := elgamal.Decrypt(priv, ciphertext)
	require.NoError(t, err)
	require.Equal(t, []byte("candidate-A"), plaintext)

	// kryptology blind signature issue
	issuer := cryptoinfra.NewKryptologyCredentialIssuer()
	credential, err := issuer.Issue(context.Background(), "user-1", []byte("user-public-key"))
	require.NoError(t, err)
	require.NotEmpty(t, credential.Signature)
}

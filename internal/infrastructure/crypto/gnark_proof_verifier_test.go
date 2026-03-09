package crypto

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"anonvote/internal/domain/entity"

	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark/backend/groth16"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/frontend/cs/r1cs"
	"github.com/stretchr/testify/require"
)

type cubicCircuit struct {
	X frontend.Variable
	Y frontend.Variable `gnark:",public"`
}

func (c *cubicCircuit) Define(api frontend.API) error {
	x3 := api.Mul(c.X, c.X, c.X)
	api.AssertIsEqual(c.Y, api.Add(x3, c.X, 5))
	return nil
}

func TestGnarkProofVerifier_Verify_Success(t *testing.T) {
	_, proofBytes, publicWitnessBytes, vkPath := setupGnarkArtifacts(t, 3, 35)

	verifier, err := NewGnarkProofVerifier(vkPath)
	require.NoError(t, err)

	err = verifier.Verify(&entity.Proof{Data: proofBytes}, entity.PublicInputs{
		"public_witness": publicWitnessBytes,
	})
	require.NoError(t, err)
}

func TestGnarkProofVerifier_Verify_InvalidWitness(t *testing.T) {
	_, proofBytes, publicWitnessBytes, vkPath := setupGnarkArtifacts(t, 3, 35)
	publicWitnessBytes[len(publicWitnessBytes)-1] ^= 0x01

	verifier, err := NewGnarkProofVerifier(vkPath)
	require.NoError(t, err)

	err = verifier.Verify(&entity.Proof{Data: proofBytes}, entity.PublicInputs{
		"public_witness": publicWitnessBytes,
	})
	require.Error(t, err)
}

func TestGnarkProofVerifier_Verify_RequiresPublicWitnessWhenVKLoaded(t *testing.T) {
	_, proofBytes, _, vkPath := setupGnarkArtifacts(t, 3, 35)

	verifier, err := NewGnarkProofVerifier(vkPath)
	require.NoError(t, err)

	err = verifier.Verify(&entity.Proof{Data: proofBytes}, entity.PublicInputs{
		"proposal_id": []byte("p-1"),
		"nullifier":   []byte("n-1"),
	})
	require.Error(t, err)
}

func setupGnarkArtifacts(
	t *testing.T,
	x int,
	y int,
) (groth16.ProvingKey, []byte, []byte, string) {
	t.Helper()

	var circuit cubicCircuit
	ccs, err := frontend.Compile(ecc.BN254, r1cs.NewBuilder, &circuit)
	require.NoError(t, err)

	pk, vk, err := groth16.Setup(ccs)
	require.NoError(t, err)

	assignment := cubicCircuit{X: x, Y: y}
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

	return pk, proofBuf.Bytes(), publicWitnessBytes, vkPath
}

func TestGnarkProofVerifier_NewWithEmptyPath_AllowsDevelopmentMode(t *testing.T) {
	verifier, err := NewGnarkProofVerifier("")
	require.NoError(t, err)

	err = verifier.Verify(&entity.Proof{Data: []byte("invalid-binary-proof")}, entity.PublicInputs{
		"public_witness": []byte("foo"),
	})
	require.NoError(t, err)
}

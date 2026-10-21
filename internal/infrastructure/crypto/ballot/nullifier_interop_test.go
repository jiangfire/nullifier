package ballot

import (
	"encoding/hex"
	"testing"

	"anonvote/internal/domain/entity"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/stretchr/testify/require"
)

// 这些向量是**跨语言**的已知答案：浏览器端
// （web/src/lib/nullifier.test.ts 与 web/src/lib/mimc.test.ts）断言的是同一批
// 值。两边任一侧改了 SHA-256、MiMC 轮常量或域运算，都会在这里对不上。
//
// 改任一侧的实现时，必须重新生成这组向量并同步两处测试。
func TestKAT_NullifierDerivation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		proposalID  string
		secret      string
		proposalHex string
		nullifierHx string
	}{
		{
			name:        "proposal-1 / secret 0x01",
			proposalID:  "proposal-1",
			secret:      "01",
			proposalHex: "22e971ef187286f3238ccf7f6552a1605434b5fc3684ef3b642cf011166b253f",
			nullifierHx: "233e35c4c857f6cb21a2942fcd0d69212e5fd917b38270a3798ae6a70f97065c",
		},
		{
			name:        "proposal-1 / secret 0x02",
			proposalID:  "proposal-1",
			secret:      "02",
			proposalHex: "22e971ef187286f3238ccf7f6552a1605434b5fc3684ef3b642cf011166b253f",
			nullifierHx: "15777d64eefe8d60ab7b54adef75f9b26ce9630546484a839553dfb5d5415021",
		},
		{
			name:        "proposal-2 / secret 0x9f",
			proposalID:  "proposal-2",
			secret:      "9f",
			proposalHex: "21e6dfa5b112b82f7a3e614744ccf2ce8335cf8bb9979832a17683cb3a96fafd",
			nullifierHx: "11c2a33748f724a0ea65b1c585881fb1d354dccb1cc3e6b69e1d37bc98c8f372",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			proposalField := DeriveProposalField(tc.proposalID)
			require.Equal(t, tc.proposalHex, EncodeScalarHex(proposalField))

			secretBytes, err := hex.DecodeString(tc.secret)
			require.NoError(t, err)
			var secret fr.Element
			secret.SetBytes(secretBytes)

			nullifier := DeriveNullifier(secret, proposalField)
			require.Equal(t, tc.nullifierHx, EncodeScalarHex(nullifier))
		})
	}
}

// MiMC 轮常量由「Keccak-256 链」派生，浏览器端必须用同一套常量。
//
// 这里把首尾两个常量钉住：常量表一旦与浏览器端错开，nullifier 就会两边
// 算出不同的值，电路的 nullifier 约束随之失效。
func TestKAT_MimcConstants(t *testing.T) {
	t.Parallel()

	constants := mimcConstants
	require.Len(t, constants, 91)

	first := fr.Element{}
	first.SetBigInt(&constants[0])
	require.Equal(
		t,
		"00808370c37267481fb91b077899955706f209e5e0762dac2c79ba1e7a91b018",
		EncodeScalarHex(first),
	)

	last := fr.Element{}
	last.SetBigInt(&constants[len(constants)-1])
	require.Equal(
		t,
		"0ad024352c40ea93df089e7950a8ef31949ca31185ea5554e507f1f5b71a82a8",
		EncodeScalarHex(last),
	)
}

// TestKAT_BrowserNullifierSatisfiesCircuit 把整条链闭合：
//
//	浏览器端 deriveNullifier（web/src/lib/nullifier.ts）
//	  == Go 侧 DeriveNullifier（上面的向量测试）
//	  == 电路里的 MiMC 约束（这里真的出一份证明并验过）
//
// 也就是说：浏览器算出来的 nullifier，电路认。
func TestKAT_BrowserNullifierSatisfiesCircuit(t *testing.T) {
	t.Parallel()

	pkPath, vkPath := writeTestKeys(t)
	prover, err := NewProver(pkPath)
	require.NoError(t, err)
	verifier, err := NewVerifier(vkPath)
	require.NoError(t, err)

	keys, err := GenerateKeyPair()
	require.NoError(t, err)

	const proposalID = "proposal-1"
	const numOptions = 3
	ciphertext, nonce, err := EncryptBallot(keys.Public, 1)
	require.NoError(t, err)

	// 浏览器端算出来的秘密与 nullifier（与 web/src/lib/nullifier.test.ts 同一组值）。
	secretBytes, err := hex.DecodeString("01")
	require.NoError(t, err)
	var secret fr.Element
	secret.SetBytes(secretBytes)
	nullifier := DeriveNullifier(secret, DeriveProposalField(proposalID))
	require.Equal(
		t,
		"233e35c4c857f6cb21a2942fcd0d69212e5fd917b38270a3798ae6a70f97065c",
		EncodeScalarHex(nullifier),
	)

	inputs, err := BuildPublicInputs(entity.BallotStatement{
		ProposalID:        proposalID,
		Nullifier:         scalarBytes(nullifier),
		ElectionPublicKey: keys.Public,
		Ciphertext:        ciphertext,
		NumOptions:        numOptions,
	})
	require.NoError(t, err)

	proof, err := prover.Prove(inputs, 1, nonce, secret)
	require.NoError(t, err)
	require.NoError(t, verifier.Verify(proof, inputs))
}

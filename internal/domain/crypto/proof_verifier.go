package crypto

import "anonvote/internal/domain/entity"

type ProofVerifier interface {
	Verify(proof *entity.Proof, inputs entity.PublicInputs) error
}

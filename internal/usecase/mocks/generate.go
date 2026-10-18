package mocks

// 版本不写死在 directive 上：mockgen 作为 go.mod 的直接依赖解析，
// 保证离线（模块缓存已命中）也能生成，且版本始终与项目依赖一致。
//go:generate go run github.com/golang/mock/mockgen -source=../../domain/repository/vote_repo.go -destination=vote_repository_mock.go -package=mocks
//go:generate go run github.com/golang/mock/mockgen -source=../../domain/repository/participation_repo.go -destination=participation_repository_mock.go -package=mocks
//go:generate go run github.com/golang/mock/mockgen -source=../../domain/repository/credential_repo.go -destination=credential_repository_mock.go -package=mocks
//go:generate go run github.com/golang/mock/mockgen -source=../../domain/repository/proposal_repo.go -destination=proposal_repository_mock.go -package=mocks
//go:generate go run github.com/golang/mock/mockgen -source=../../domain/repository/decryption_session_repo.go -destination=decryption_session_repository_mock.go -package=mocks
//go:generate go run github.com/golang/mock/mockgen -source=../../domain/crypto/proof_verifier.go -destination=proof_verifier_mock.go -package=mocks
//go:generate go run github.com/golang/mock/mockgen -source=../../domain/crypto/encrypter.go -destination=encrypter_mock.go -package=mocks

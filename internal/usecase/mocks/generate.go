package mocks

//go:generate go run github.com/golang/mock/mockgen@v1.6.0 -source=../../domain/repository/vote_repo.go -destination=vote_repository_mock.go -package=mocks
//go:generate go run github.com/golang/mock/mockgen@v1.6.0 -source=../../domain/repository/participation_repo.go -destination=participation_repository_mock.go -package=mocks
//go:generate go run github.com/golang/mock/mockgen@v1.6.0 -source=../../domain/repository/credential_repo.go -destination=credential_repository_mock.go -package=mocks
//go:generate go run github.com/golang/mock/mockgen@v1.6.0 -source=../../domain/crypto/proof_verifier.go -destination=proof_verifier_mock.go -package=mocks
//go:generate go run github.com/golang/mock/mockgen@v1.6.0 -source=../../domain/crypto/encrypter.go -destination=encrypter_mock.go -package=mocks

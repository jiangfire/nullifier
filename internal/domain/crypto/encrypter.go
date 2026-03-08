package crypto

type Encrypter interface {
	ValidateCiphertext(cipher []byte) error
}

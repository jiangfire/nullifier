package entity

// Proof 封装零知识证明字节流。
type Proof struct {
	Data []byte `json:"data"`
}

// PublicInputs 为验证证明所需公开输入。
type PublicInputs map[string][]byte

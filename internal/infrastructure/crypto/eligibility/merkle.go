package eligibility

import (
	"bytes"
	"encoding/hex"
	"fmt"

	"anonvote/internal/infrastructure/crypto/mimc"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
)

// FieldSize 是域元素的字节数（BN254 标量域的规范编码长度）。
const FieldSize = 32

// MemberPackage 是发给成员本人的资格包。
//
// 它是**秘密材料**：拿到它就能领这一票。名单工具的输出目录必须与 root 分开
// 保管（见 docs/t22-eligibility-design.md §3.1）。
type MemberPackage struct {
	// MemberID 是成员标识（工号、选民编号……）。它只出现在成员自己的包里，
	// 服务端看不到。
	MemberID string `json:"member_id"`
	// Secret 是资格秘密（32 字节随机值的十六进制，规范编码）。
	Secret string `json:"secret"`
	// Leaf 是 MiMC(Secret)，即这枚资格在名单树里的叶子。
	Leaf string `json:"leaf"`
	// Index 是叶子在树里的位置。
	Index uint64 `json:"index"`
	// Path 是从叶子到根的 Merkle 路径（兄弟节点，十六进制）。
	Path []string `json:"path"`
	// Root 是名单树根 —— 服务端配置里的那个值。
	Root string `json:"root"`
	// Depth 是树深。
	Depth int `json:"depth"`
}

// ListPackage 是给服务端的名单根。
//
// 服务端**只需要**它：有了 root，服务端就能验证「这个人的叶子在树里」，
// 却无法把叶子对应到任何成员（它没有成员包，也没有名单）。
type ListPackage struct {
	// Root 是名单树根（十六进制）。
	Root string `json:"root"`
	// Depth 是树深。
	Depth int `json:"depth"`
	// Count 是名单里的成员数（不含补齐的空叶子）。
	Count int `json:"count"`
	// Label 是运营方给名单标注的来源说明（可选，v0.4.0 起随 -label 写入）。
	// 服务端不读它 —— 这是给「管名单的人」看的。
	Label string `json:"label,omitempty"`
	// GeneratedAt 是名单生成时间（RFC3339，UTC）。
	GeneratedAt string `json:"generated_at,omitempty"`
}

// DeriveLeaf 由资格秘密推出叶子：leaf = MiMC(secret)。
func DeriveLeaf(secret []byte) (fr.Element, error) {
	field, err := SecretToField(secret)
	if err != nil {
		return fr.Element{}, err
	}
	return mimc.Hash(field), nil
}

// SecretToField 把资格秘密（32 字节）转成域元素。
//
// 注意是**归约**而不是拒绝：随机 32 字节有约 2^-32 的概率超过模数。归约后
// 的规范编码才是真正被电路使用的秘密，成员包里存的必须是它 —— 否则客户端
// 与名单工具算出的叶子会不一致。
func SecretToField(secret []byte) (fr.Element, error) {
	if len(secret) != FieldSize {
		return fr.Element{}, fmt.Errorf("invalid secret length: %d", len(secret))
	}
	var field fr.Element
	field.SetBytes(secret)
	return field, nil
}

// CanonicalSecret 把任意 32 字节的秘密归一成规范编码。
func CanonicalSecret(secret []byte) ([]byte, error) {
	field, err := SecretToField(secret)
	if err != nil {
		return nil, err
	}
	bytes := field.Bytes()
	return bytes[:], nil
}

// DeriveQualificationID 推导去重键：MiMC(leaf, 提案域元素)。
//
// 同一成员在同一提案里得到同一个值（第二次领取会被唯一索引拦下），
// 不同提案得到不同的值（跨提案不可关联）。
func DeriveQualificationID(leaf fr.Element, proposalField fr.Element) fr.Element {
	return mimc.Hash(leaf, proposalField)
}

// BuildTree 由叶子列表构建 Merkle 树，返回树根与每个叶子的路径。
//
// 叶子数不足 2^depth 时用零叶子补齐 —— 补齐的叶子不对应任何成员，
// 也没有人能证明自己持有它们的秘密（除非拿到那个零秘密）。
func BuildTree(leaves []fr.Element, depth int) (fr.Element, [][]fr.Element, error) {
	if err := ValidateTreeDepth(depth); err != nil {
		return fr.Element{}, nil, err
	}
	size := 1 << depth
	if len(leaves) > size {
		return fr.Element{}, nil, fmt.Errorf("too many leaves: %d > %d", len(leaves), size)
	}

	level := make([]fr.Element, size)
	copy(level, leaves)
	for i := len(leaves); i < size; i++ {
		level[i] = fr.Element{}
	}

	// 逐层上溯，每层都记下来以便回溯路径。
	levels := make([][]fr.Element, depth+1)
	levels[0] = level
	for d := 0; d < depth; d++ {
		current := levels[d]
		parent := make([]fr.Element, len(current)/2)
		for j := range parent {
			parent[j] = mimc.Hash(current[2*j], current[2*j+1])
		}
		levels[d+1] = parent
	}

	root := levels[depth][0]
	paths := make([][]fr.Element, len(leaves))
	for i := range leaves {
		path := make([]fr.Element, depth)
		position := i
		for d := 0; d < depth; d++ {
			path[d] = levels[d][position^1]
			position /= 2
		}
		paths[i] = path
	}
	return root, paths, nil
}

// WalkPath 由叶子沿 Merkle 路径上溯，返回推导出的根。
//
// 走法与电路一致：第 i 层的左右由 index 第 i 位决定（0 = 当前节点是左孩子，
// 1 = 右孩子），父节点 = MiMC(左, 右)。它给离线校验用 —— 成员或运营方可以
// 不跑电路就核对「这条路径真的通到这个根」。
func WalkPath(leaf fr.Element, index uint64, path []fr.Element) (fr.Element, error) {
	node := leaf
	for i, sibling := range path {
		bit := (index >> uint(i)) & 1
		if bit == 0 {
			node = mimc.Hash(node, sibling)
		} else {
			node = mimc.Hash(sibling, node)
		}
	}
	return node, nil
}

// EncodeField 把域元素编码成十六进制（规范形式）。
func EncodeField(element fr.Element) string {
	bytes := element.Bytes()
	return hex.EncodeToString(bytes[:])
}

// decodeHex 解析十六进制字节串。
func decodeHex(value string) ([]byte, error) {
	raw, err := hex.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("decode hex: %w", err)
	}
	return raw, nil
}

// DecodeField 解析十六进制域元素。
//
// 与 `ballot` 侧的口径一致：SetBytes 是**归约**而不是拒绝，因此这里额外检查
// 规范编码 —— 非规范编码一律拒绝，避免同一个值有两种字节表示。
func DecodeField(value string) (fr.Element, error) {
	raw, err := hex.DecodeString(value)
	if err != nil {
		return fr.Element{}, fmt.Errorf("decode field element: %w", err)
	}
	if len(raw) != FieldSize {
		return fr.Element{}, fmt.Errorf("invalid field element length: %d", len(raw))
	}
	var element fr.Element
	element.SetBytes(raw)
	canonical := element.Bytes()
	if !bytes.Equal(canonical[:], raw) {
		return fr.Element{}, fmt.Errorf("non-canonical field element encoding")
	}
	return element, nil
}

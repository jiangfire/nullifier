// Package eligibility 实现「资格证明」：投票人证明「我在资格名单里」，而不
// 让服务端知道他是谁。
//
// 背景见 [`docs/t22-eligibility-design.md`](../../../docs/t22-eligibility-design.md)：
// 服务端要强制「一人一票」，就必须能对某个标识去重；而只要这个标识与真人
// 的对应关系是服务端可见的，去重就等于给了服务端关联票与人的能力。
// 唯一能同时拿到「一人一票」与「不可关联」的做法是：资格由权威名单分配，
// 而名单里的成员以**零知识**的方式出示资格。
//
// 本包的四件东西：
//
//  1. 名单与 Merkle 承诺（merkle.go）：离线工具把成员名单变成一棵 Merkle 树，
//     服务端只拿到树根 root，成员各自拿到「秘密 + 叶子 + 路径」。
//  2. 资格电路（circuit.go）：证明「我持有某个秘密，它的叶子在这棵树的根下」。
//  3. 公开输入（statement.go）：root、提案、挑战、去重键、会话绑定 —— 五个定长元素。
//  4. 证明器（prover.go）：Groth16 的 prover / verifier 与密钥生成。
//
// **去重键**是 `QualificationID = MiMC(Leaf, ProposalID)`：同一成员在同一提案里
// 永远得到同一个值（所以第二次领取会被唯一索引拦下），而不同提案之间互不相同
// （所以跨提案不可关联）。
package eligibility

import (
	"fmt"

	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/std/hash/mimc"
)

// DefaultTreeDepth 是资格名单 Merkle 树的**默认**深度。
//
// 历史口径：v0.2.0 起电路就按深度 10 编译，存量名单与密钥都是它 —— keygen、
// 服务端与客户端在「没有指定深度」时一律取这个值。深度是**部署决策**
// （v0.4.0 起）：换更深（或更浅）的名单要重新生成密钥与名单，并把 root /
// verifying key / depth 三项成套配到服务端。
const DefaultTreeDepth = 10

// 树深允许的范围。
//
// 下限防呆：2^4 = 16 人以下就谈不上「名单」了。上限由电路规模自然兜底 ——
// 约束数与 Merkle 上溯层数随深度线性增长，密钥生成随之变慢；2^24 ≈ 1678 万
// 成员，远超任何单份名单的需要。
const (
	MinTreeDepth = 4
	MaxTreeDepth = 24
)

// ValidateTreeDepth 检查名单树深是否在支持范围内。
func ValidateTreeDepth(depth int) error {
	if depth < MinTreeDepth || depth > MaxTreeDepth {
		return fmt.Errorf("eligibility tree depth must be between %d and %d, got %d",
			MinTreeDepth, MaxTreeDepth, depth)
	}
	return nil
}

// eligibilityCircuit 是资格证明的零知识电路。
//
// 公开输入（服务端自拼，客户端无从篡改）：
//
//	Root             资格名单的 Merkle 树根
//	ProposalID       提案 ID 映射出的域元素
//	Challenge        本次领取会话的一次性挑战
//	QualificationID  去重键 = MiMC(Leaf, ProposalID)
//	SessionBinding   = MiMC(QualificationID, Challenge)
//
// 私有见证：
//
//	Secret  资格秘密（成员包里的 32 字节随机值）
//	Index   叶子在树里的位置
//	Path    Merkle 路径（从叶子到根的兄弟节点）
//
// 电路断言四件事：
//
//  1. Leaf = MiMC(Secret) —— 叶子由秘密单向决定，知道叶子推不出秘密；
//  2. QualificationID = MiMC(Leaf, ProposalID) —— 去重键绑定本提案；
//  3. SessionBinding = MiMC(QualificationID, Challenge) —— 证明绑定本次领取
//     会话的挑战，于是「截获的证明」不能挪到别的挑战上用；
//  4. 从 Leaf 沿 Path 上溯得到 Root —— 叶子确实在这棵名单树里。
//
// 路径与叶子都是**私有**的：服务端验证时只需要上面五个公开输入 ——
// 公开输入的个数因此是定长的，不随树深变化。
type eligibilityCircuit struct {
	// 公开输入。字段顺序即 witness 的填充顺序，改动会让已签发密钥失效。
	Root            frontend.Variable `gnark:",public"`
	ProposalID      frontend.Variable `gnark:",public"`
	Challenge       frontend.Variable `gnark:",public"`
	QualificationID frontend.Variable `gnark:",public"`
	SessionBinding  frontend.Variable `gnark:",public"`

	// 私有见证。
	Secret frontend.Variable
	Index  frontend.Variable
	Path   []frontend.Variable
}

// Define 描述资格电路的约束。
func (c *eligibilityCircuit) Define(api frontend.API) error {
	// 1) leaf = MiMC(secret)
	leafHash, err := mimc.NewMiMC(api)
	if err != nil {
		return fmt.Errorf("init mimc: %w", err)
	}
	leafHash.Write(c.Secret)
	leaf := leafHash.Sum()

	// 2) 去重键 = MiMC(leaf, 提案 ID)
	qualificationHash, err := mimc.NewMiMC(api)
	if err != nil {
		return fmt.Errorf("init mimc: %w", err)
	}
	qualificationHash.Write(leaf, c.ProposalID)
	api.AssertIsEqual(qualificationHash.Sum(), c.QualificationID)

	// 3) 会话绑定 = MiMC(去重键, 挑战)
	//    挑战因此被钉在这份证明上：截获的证明挪不到别的挑战上用。
	//    去重键本身不含挑战 —— 它必须对同一成员在同一提案里保持稳定，
	//    否则唯一索引拦不住重复领取。
	bindingHash, err := mimc.NewMiMC(api)
	if err != nil {
		return fmt.Errorf("init mimc: %w", err)
	}
	bindingHash.Write(c.QualificationID, c.Challenge)
	api.AssertIsEqual(bindingHash.Sum(), c.SessionBinding)

	// 4) index 的位分解：宽度取 Path 的长度（即本实例编译时的树深），且能复原回
	//    index 本身，否则「选左还是选右」这一位就与位置脱钩了。
	//    深度由编译模板的 Path 切片长度决定（见 compileCircuit），不是编译期常量。
	depth := len(c.Path)
	if err := ValidateTreeDepth(depth); err != nil {
		return fmt.Errorf("merkle path length out of range: %w", err)
	}
	bits := api.ToBinary(c.Index, depth)
	var rebuilt frontend.Variable = frontend.Variable(0)
	for i := depth - 1; i >= 0; i-- {
		rebuilt = api.Add(api.Mul(rebuilt, 2), bits[i])
	}
	api.AssertIsEqual(rebuilt, c.Index)

	// 5) Merkle 上溯：node = MiMC(left, right)，左右由 index 的第 i 位决定。
	node := leaf
	for i := 0; i < depth; i++ {
		sibling := c.Path[i]
		// 位为 0 → 当前节点是左孩子；位为 1 → 当前节点是右孩子。
		left := api.Select(bits[i], sibling, node)
		right := api.Select(bits[i], node, sibling)

		levelHash, err := mimc.NewMiMC(api)
		if err != nil {
			return fmt.Errorf("init mimc: %w", err)
		}
		levelHash.Write(left, right)
		node = levelHash.Sum()
	}
	api.AssertIsEqual(node, c.Root)

	return nil
}

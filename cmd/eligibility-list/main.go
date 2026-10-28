// Command eligibility-list 由权威成员名单生成资格名单的 Merkle 承诺。
//
// 它是**离线工具**：不联网、不连服务端，跑在运营方（或独立的登记处）的机器上。
//
// 产物有两份，**必须分开保管**：
//
//	root.json            名单树根 —— 部署到投票服务端（crypto.eligibility_root）
//	eligibility-*.json   每位成员一份资格包（秘密 + 叶子 + 路径）—— 走安全渠道发给本人
//
// 服务端只拿到 root：它能验证「这个人的叶子在这棵名单树里」，却**无法把叶子
// 对应到任何成员**（它没有成员包，也没有名单）。这是能力 G（分发不泄露身份）
// 与「一人一票」能同时成立的关键。
//
// 名单来源（v0.4.0 起）：`.txt` 每行一个成员标识；`.csv` 走表头解析
// （默认取第一列，`-id-column` 指定列名；UTF-8 BOM 容错 —— Excel 导出常态）。
// **重复成员默认拒绝**：真实名单里混入重复行时，静默去重会让一个成员悄悄
// 失去资格且无人知道 —— 宁可拒绝生成，让人先清洗名单。
//
// 生成之后用 `-verify <目录>` 离线校验产出：逐个成员包从秘密重导叶子、沿路径
// 上溯到根，与 root.json 比对 —— 退出码 0 / 1 / 2 可当门禁用。
//
// 用法：
//
//	go run ./cmd/eligibility-list -members members.txt -out ./eligibility-out
//	go run ./cmd/eligibility-list -members roster.csv -id-column 工号 -out ./out -label "2026 年度大会"
//	go run ./cmd/eligibility-list -verify ./eligibility-out
package main

import (
	"bufio"
	"crypto/rand"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"anonvote/internal/infrastructure/crypto/eligibility"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

// run 解析参数并执行，返回进程退出码。
func run(args []string) int {
	fs := flag.NewFlagSet("nullifier-eligibility-list", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	membersPath := fs.String("members", "", "member list file (.txt: one id per line; .csv: header row + -id-column)")
	outDir := fs.String("out", "./eligibility-out", "directory to write the list root and member packages into")
	// 名单树的深度。它决定名单规模上限（2^depth 人），且 root / 验证密钥 /
	// 深度必须与服务端成套 —— 换深度等于换名单换密钥。
	depth := fs.Int("depth", eligibility.DefaultTreeDepth,
		"Merkle tree depth; the list can hold at most 2^depth members")
	force := fs.Bool("force", false, "overwrite existing files")
	label := fs.String("label", "", "free-form roster label recorded in root.json (roster source, election name, ...)")
	idColumn := fs.String("id-column", "", "CSV column holding the member id (default: the first column)")
	verifyDir := fs.String("verify", "", "verify an existing output directory instead of generating a list")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprintln(os.Stdout, "用法: nullifier-eligibility-list -members <名单文件> [-depth <树深>] [-out <目录>] [-force] [-label <标注>] [-id-column <CSV 列名>]")
			fmt.Fprintln(os.Stdout, "      nullifier-eligibility-list -verify <产出目录>")
			fs.SetOutput(os.Stdout)
			fs.PrintDefaults()
			return 0
		}
		fmt.Fprintf(os.Stderr, "参数错误: %v\n", err)
		return 2
	}

	// 校验模式：不生成，只核对一份已有的产出。
	if strings.TrimSpace(*verifyDir) != "" {
		if err := verifyList(*verifyDir); err != nil {
			fmt.Fprintf(os.Stderr, "校验未通过: %v\n", err)
			return 1
		}
		fmt.Fprintf(os.Stdout, "校验通过: %s\n", *verifyDir)
		return 0
	}

	if strings.TrimSpace(*membersPath) == "" {
		fmt.Fprintln(os.Stderr, "必须提供 -members（或用 -verify 校验已有产出）")
		return 2
	}
	if err := eligibility.ValidateTreeDepth(*depth); err != nil {
		fmt.Fprintf(os.Stderr, "参数错误: %v\n", err)
		return 2
	}

	memberIDs, err := readMembers(*membersPath, *idColumn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取名单失败: %v\n", err)
		return 1
	}
	if len(memberIDs) == 0 {
		fmt.Fprintln(os.Stderr, "名单是空的")
		return 1
	}

	if err := generateList(memberIDs, *outDir, *depth, *label, *force); err != nil {
		fmt.Fprintf(os.Stderr, "生成名单失败: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stdout, "资格名单已生成: %d 名成员, 树深 %d（规模上限 %d 人）\n",
		len(memberIDs), *depth, 1<<uint(*depth))
	fmt.Fprintf(os.Stdout, "名单树根（部署到服务端 crypto.eligibility_root）: %s\n",
		filepath.Join(*outDir, "root.json"))
	fmt.Fprintf(os.Stdout, "成员资格包: %s/eligibility-<member_id>.json —— 逐个走安全渠道发给本人\n", *outDir)
	fmt.Fprintf(os.Stdout, "发完之后用 -verify %s 核对产出成套\n", *outDir)
	return 0
}

// readMembers 读名单文件，返回排序去重后的成员标识。
//
// `.csv` 走表头解析：首行是表头，成员 ID 列由 -id-column 指定（缺省第一列）；
// 其余按纯文本处理（每行一个标识，空行与 # 开头的行忽略）。UTF-8 BOM 容错。
//
// **重复成员是错误**，不是可容忍的输入：静默去重会让一个成员悄悄失去资格
// 且无人知道 —— 名单少一个人比生成失败严重得多。报错列出全部重复项，
// 让运营方先清洗名单。
func readMembers(path string, idColumn string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open members file: %w", err)
	}
	defer func() { _ = file.Close() }()

	var (
		seen    = map[string]int{}
		members = make([]string, 0, 64)
	)

	if strings.EqualFold(filepath.Ext(path), ".csv") {
		reader := csv.NewReader(bufio.NewReader(file))
		// Excel 导出常见 BOM；csv 不认它，会把第一列的列名读成 "\ufeffid"。
		reader.FieldsPerRecord = -1 // 列数按行容错，缺列在下面显式检查。
		records, err := reader.ReadAll()
		if err != nil {
			return nil, fmt.Errorf("read csv: %w", err)
		}
		if len(records) == 0 {
			return nil, errors.New("csv file is empty")
		}
		header := records[0]
		for i := range header {
			// Excel 导出常在文件头放 UTF-8 BOM，会粘在第一个列表名上。
			header[i] = strings.TrimPrefix(strings.TrimSpace(header[i]), "\ufeff")
		}
		column := 0
		if strings.TrimSpace(idColumn) != "" {
			column = -1
			for i, name := range header {
				if name == strings.TrimSpace(idColumn) {
					column = i
					break
				}
			}
			if column < 0 {
				return nil, fmt.Errorf("column %q not found in csv header (%s)",
					idColumn, strings.Join(header, ", "))
			}
		} else if len(header) == 0 {
			return nil, errors.New("csv header is empty")
		}
		for _, record := range records[1:] {
			if column >= len(record) {
				return nil, fmt.Errorf("csv row %s is missing the id column", strings.Join(record, ","))
			}
			id := strings.TrimSpace(record[column])
			if id == "" {
				continue
			}
			members = append(members, id)
		}
	} else {
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			members = append(members, line)
		}
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("read members file: %w", err)
		}
	}

	// 重复检查放在格式解析之后、写盘之前：报错要列出全部重复项。
	for _, id := range members {
		if first, duplicated := seen[id]; duplicated {
			return nil, fmt.Errorf(
				"名单里有重复成员 %q（第 %d 行与第 %d 行）；请先清洗名单 —— "+
					"静默去重会让一个成员悄悄失去资格",
				id, first+1, len(members),
			)
		}
		seen[id] = len(members)
	}

	// 排序让成员的位置是**确定的**：同一份名单每次生成时成员的叶子位置一致。
	// 但资格秘密每次运行都会重新随机生成，所以**两次运行的树根不一样** ——
	// 名单与成员包必须成套保存，不能只留 root。
	sort.Strings(members)
	return members, nil
}

// generateList 生成树根与成员包并落盘。
func generateList(memberIDs []string, outDir string, depth int, label string, force bool) error {
	// 名单装不下要在**写任何文件之前**说清楚，并且告诉运营方出路 ——
	// 深度决定规模上限（2^depth），装不下就换更深的树重新生成
	// （密钥也要跟着换：每深度一套密钥）。
	if capacity := 1 << uint(depth); len(memberIDs) > capacity {
		return fmt.Errorf(
			"名单有 %d 名成员，超过树深 %d 的容量上限 %d 人；用 -depth 换更深的树（并重新生成配套密钥）",
			len(memberIDs), depth, capacity,
		)
	}

	// 撞名预检必须在**写任何文件之前**做完。
	//
	// 成员包的文件名由成员标识归一化而来（sanitizeFileName），而归一化会把
	// 字母数字与 `-_.` 之外的字符一律换成 `_`。于是「张三-01」与「李四-01」
	// 会归一成同一个文件名 —— 两个不同成员共用一个包名，后写的那个会把
	// 前一个**静默覆盖**（带 -force 时）或让整次生成半途失败（不带时）。
	// 后者更糟：root.json 已经落盘、成员包只写了一半，root 声称的成员数与
	// 实际包数对不上；运营方若看到报错就加 -force 重跑，缺的包就永远消失了。
	//
	// 「谁拿到谁就能领这一票」—— 张三-01 若拿到李四-01 的包，两个人就能领
	// 同一票，一人一票当场失效。所以这里**宁可拒绝生成，也不产出无法区分的包**。
	if err := checkMemberFileNames(memberIDs); err != nil {
		return err
	}

	secrets := make([][]byte, len(memberIDs))
	leaves := make([]fr.Element, len(memberIDs))
	for i := range memberIDs {
		secret := make([]byte, eligibility.FieldSize)
		if _, err := rand.Read(secret); err != nil {
			return fmt.Errorf("generate secret: %w", err)
		}
		canonical, err := eligibility.CanonicalSecret(secret)
		if err != nil {
			return fmt.Errorf("canonicalize secret: %w", err)
		}
		secrets[i] = canonical

		leaf, err := eligibility.DeriveLeaf(canonical)
		if err != nil {
			return fmt.Errorf("derive leaf: %w", err)
		}
		leaves[i] = leaf
	}

	root, paths, err := eligibility.BuildTree(leaves, depth)
	if err != nil {
		return fmt.Errorf("build tree: %w", err)
	}
	rootHex := eligibility.EncodeField(root)

	if err := os.MkdirAll(outDir, 0o700); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	list := eligibility.ListPackage{
		Root:        rootHex,
		Depth:       depth,
		Count:       len(memberIDs),
		Label:       strings.TrimSpace(label),
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
	}

	// 成员包先写，root.json **最后**写。
	//
	// root.json 是要配到服务端的那一份 —— 它落盘就意味着「这份名单可以上线」。
	// 如果先写 root，写成员包时中途失败，目录里就留下一份 root 声称 N 人、
	// 实际只有 N-1 个包的名单；运营方把 root 配上去，服务端就会按一个不完整的
	// 名单工作。反过来（包先写、root 最后）失败时目录里没有 root，
	// 也就是「这份名单还没准备好」，不会误上线。
	for i, memberID := range memberIDs {
		path := make([]string, len(paths[i]))
		for j := range paths[i] {
			path[j] = eligibility.EncodeField(paths[i][j])
		}
		pack := eligibility.MemberPackage{
			MemberID: memberID,
			Secret:   hexString(secrets[i]),
			Leaf:     eligibility.EncodeField(leaves[i]),
			Index:    uint64(i),
			Path:     path,
			Root:     rootHex,
			Depth:    depth,
		}
		name := memberFileName(memberID)
		if err := writeJSON(filepath.Join(outDir, name), pack, 0o600, force); err != nil {
			return err
		}
	}

	if err := writeJSON(filepath.Join(outDir, "root.json"), list, 0o644, force); err != nil {
		return err
	}
	return nil
}

// verifyList 离线校验一份产出目录。
//
// 核对四件事：① root.json 可解析且深度合法；② 每个成员包的叶子由它自己的
// 秘密推出（包没被改过）；③ 每条路径沿包里记的位置上溯，真的通到 root.json
// 的根（路径没有被换过）；④ 包数与 root.json 声称的成员数一致（没丢包）。
// 任何一条不满足都返回错误 —— 退出码非 0，可当门禁用。
func verifyList(dir string) error {
	raw, err := os.ReadFile(filepath.Join(dir, "root.json"))
	if err != nil {
		return fmt.Errorf("read root.json: %w", err)
	}
	var list eligibility.ListPackage
	if err := json.Unmarshal(raw, &list); err != nil {
		return fmt.Errorf("parse root.json: %w", err)
	}
	if err := eligibility.ValidateTreeDepth(list.Depth); err != nil {
		return fmt.Errorf("root.json: %w", err)
	}
	expectedRoot, err := eligibility.DecodeField(list.Root)
	if err != nil {
		return fmt.Errorf("root.json: invalid root: %w", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read directory: %w", err)
	}
	verified := 0
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "eligibility-") || !strings.HasSuffix(name, ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		var pack eligibility.MemberPackage
		if err := json.Unmarshal(raw, &pack); err != nil {
			return fmt.Errorf("parse %s: %w", name, err)
		}
		if err := verifyMemberPackage(&pack, &list, expectedRoot); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		verified++
	}

	if verified != list.Count {
		return fmt.Errorf(
			"目录里有 %d 份成员包，root.json 声称 %d 名成员 —— 有包丢了或没发完，这份名单不能上线",
			verified, list.Count,
		)
	}
	if verified == 0 {
		return errors.New("目录里没有任何成员包")
	}
	return nil
}

// verifyMemberPackage 校验单个成员包的自洽性。
func verifyMemberPackage(pack *eligibility.MemberPackage, list *eligibility.ListPackage, expectedRoot fr.Element) error {
	if pack.Depth != list.Depth {
		return fmt.Errorf("package depth %d does not match list depth %d", pack.Depth, list.Depth)
	}
	if len(pack.Path) != pack.Depth {
		return fmt.Errorf("package path has %d elements, expected %d", len(pack.Path), pack.Depth)
	}
	if pack.Root != list.Root {
		return errors.New("package was issued for a different list root")
	}

	secret, err := decodeHex(pack.Secret)
	if err != nil {
		return fmt.Errorf("decode secret: %w", err)
	}
	leaf, err := eligibility.DeriveLeaf(secret)
	if err != nil {
		return fmt.Errorf("derive leaf: %w", err)
	}
	packLeaf, err := eligibility.DecodeField(pack.Leaf)
	if err != nil {
		return fmt.Errorf("decode leaf: %w", err)
	}
	if !leaf.Equal(&packLeaf) {
		return errors.New("leaf does not match the secret (package was modified)")
	}

	path := make([]fr.Element, len(pack.Path))
	for i, raw := range pack.Path {
		element, err := eligibility.DecodeField(raw)
		if err != nil {
			return fmt.Errorf("decode path element %d: %w", i, err)
		}
		path[i] = element
	}
	walked, err := eligibility.WalkPath(leaf, pack.Index, path)
	if err != nil {
		return fmt.Errorf("walk path: %w", err)
	}
	if !walked.Equal(&expectedRoot) {
		return errors.New("path does not lead to the list root")
	}
	return nil
}

// memberFileName 是成员包的文件名。
func memberFileName(memberID string) string {
	return fmt.Sprintf("eligibility-%s.json", sanitizeFileName(memberID))
}

// checkMemberFileNames 检查有没有两个成员归一化后落到同一个文件名。
//
// 归一化规则见 sanitizeFileName：字母数字与 `-_.` 之外的字符一律换成 `_`。
// 现实名单里中文标识很常见（「张三-01」「李四-01」），它们会归一成同一个名字 ——
// 共用一个包名就等于有一个人会拿不到自己的包（或者拿到别人的）。
// 这里在**写盘前**拒绝，并把撞名的成员标识与落到一起的文件名一起报出来，
// 让运营方能直接改标识或改用别的分发方式。
func checkMemberFileNames(memberIDs []string) error {
	byName := map[string][]string{}
	for _, memberID := range memberIDs {
		name := memberFileName(memberID)
		byName[name] = append(byName[name], memberID)
	}
	// 撞到的按文件名排序，报错信息才稳定。
	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		ids := byName[name]
		if len(ids) < 2 {
			continue
		}
		return fmt.Errorf(
			"成员标识 %s 归一化后都落到同一个文件 %s，无法生成可区分的资格包；"+
				"请改用只含字母数字与 -_. 的标识（或在标识里加区分后缀）",
			strings.Join(ids, "、"), name,
		)
	}
	return nil
}

// writeJSON 写 JSON 文件。
func writeJSON(path string, payload any, mode os.FileMode, force bool) error {
	if !force {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("%s already exists (use -force to overwrite)", path)
		}
	}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(path, raw, mode); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// sanitizeFileName 把成员标识变成安全的文件名。
func sanitizeFileName(memberID string) string {
	var out strings.Builder
	for _, r := range memberID {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '-', r == '_', r == '.':
			out.WriteRune(r)
		default:
			out.WriteRune('_')
		}
	}
	return out.String()
}

// hexString 把字节编成小写十六进制。
func hexString(raw []byte) string {
	const digits = "0123456789abcdef"
	out := make([]byte, 0, len(raw)*2)
	for _, b := range raw {
		out = append(out, digits[b>>4], digits[b&0x0f])
	}
	return string(out)
}

// decodeHex 解析十六进制字节串（verify 用）。
func decodeHex(value string) ([]byte, error) {
	raw, err := hex.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("decode hex: %w", err)
	}
	return raw, nil
}

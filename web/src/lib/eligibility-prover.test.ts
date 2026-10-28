import { describe, expect, it } from 'vitest';
import { isReactive, ref } from 'vue';
import { buildEligibilityProveRequest } from './eligibility-prover';

/**
 * 资格证明的**跨语言字符串**锁定。
 *
 * 这一侧只做转发：门户把成员包与挑战原样交给 WASM 模块，密码学全在 Go 侧。
 * 因此真正会出事的是**字段名**：Go 侧改了 json tag、这边没改，表现是
 * 「资格包无效」或者更糟 —— 静默地少传一个字段。
 *
 * 对端断言：`tests/architecture/eligibility_wire_test.go` 拿 Go 侧的
 * struct tag 与这份名单逐条对照。
 */
describe('资格证明的线路字段名', () => {
  it('请求体用 Go 侧的 snake_case 字段名', () => {
    const request = buildEligibilityProveRequest({
      proposalId: 'p1',
      challenge: 'challenge-hex',
      root: 'root-hex',
      memberPackage: {
        member_id: 'member-1',
        secret: 'secret',
        leaf: 'leaf',
        index: 0,
        path: ['sibling'],
        root: 'root-hex',
        depth: 10,
      },
    });

    expect(Object.keys(request).sort()).toEqual([
      'challenge',
      'member_package',
      'proposal_id',
      'root',
    ]);
    expect(request.proposal_id).toBe('p1');
    expect(request.challenge).toBe('challenge-hex');
    expect(request.root).toBe('root-hex');
  });

  it('成员包的字段名与离线工具产出的 JSON 一致', () => {
    const request = buildEligibilityProveRequest({
      proposalId: 'p1',
      challenge: 'challenge-hex',
      root: 'root-hex',
      memberPackage: {
        member_id: 'member-1',
        secret: 'secret',
        leaf: 'leaf',
        index: 3,
        path: ['a', 'b'],
        root: 'root-hex',
        depth: 10,
      },
    });

    expect(Object.keys(request.member_package).sort()).toEqual([
      'depth',
      'index',
      'leaf',
      'member_id',
      'path',
      'root',
      'secret',
    ]);
    expect(request.member_package.index).toBe(3);
    expect(request.member_package.path).toEqual(['a', 'b']);
  });
});

/**
 * 门户里资格包存在 Vue 的 `ref` 里，读出来是**响应式 Proxy**。
 *
 * `postMessage` 用结构化克隆传参，而结构化克隆克隆不了 Proxy —— 浏览器直接抛
 * `DataCloneError: #<Object> could not be cloned.`，「匿名领取」在真浏览器里必然
 * 失败。这条用例把那个坑钉住：拼出来的请求体必须是**普通对象**，且能被克隆。
 */
describe('资格证明请求体的可克隆性', () => {
  it('响应式 ref 里的成员包也能被结构化克隆', () => {
    const pack = ref({
      member_id: 'member-1',
      secret: 'secret',
      leaf: 'leaf',
      index: 3,
      path: ['a', 'b'],
      root: 'root-hex',
      depth: 10,
    });

    const request = buildEligibilityProveRequest({
      proposalId: 'p1',
      challenge: 'challenge-hex',
      root: 'root-hex',
      memberPackage: pack.value,
    });

    expect(isReactive(request.member_package)).toBe(false);
    expect(isReactive(request.member_package.path)).toBe(false);
    expect(() => structuredClone(request)).not.toThrow();
    expect(structuredClone(request).member_package.path).toEqual(['a', 'b']);
  });
});

/**
 * 默认树深的跨语言锁（TS 半边）。
 *
 * v0.4.0 起名单树深是运行期参数（4–24），但**默认值 10** 是 v0.2.0 存量名单
 * 与密钥的口径：成员包没带深度时（理论上不该有，但门户对旧包要宽容）WASM 侧
 * 取的就是它。Go 半边的断言在
 * `internal/infrastructure/crypto/eligibility/depth_test.go` 的
 * `TestDefaultTreeDepthIsTheLegacyShape` —— 两边各钉一条，单边改动必红。
 */
describe('默认树深的跨语言锁', () => {
  it('深度是数字并原样进入请求体（WASM 以它初始化电路）', () => {
    const request = buildEligibilityProveRequest({
      proposalId: 'p1',
      challenge: 'challenge-hex',
      root: 'root-hex',
      memberPackage: {
        member_id: 'member-1',
        secret: 'secret',
        leaf: 'leaf',
        index: 0,
        path: ['a', 'b'],
        root: 'root-hex',
        depth: 10,
      },
    });

    expect(request.member_package.depth).toBe(10);
    expect(typeof request.member_package.depth).toBe('number');
  });
});

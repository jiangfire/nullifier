import { describe, expect, it } from 'vitest';
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

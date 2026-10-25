import { describe, expect, it } from 'vitest';
import { assetUrl, toLocalBallotResult } from './wasm-prover';

describe('浏览器端 prover 的结果整理', () => {
  it('失败的结果抛出错误', () => {
    expect(() => toLocalBallotResult({ ok: false, error: '下载 WASM 模块失败' })).toThrow(
      '下载 WASM 模块失败',
    );
  });

  it('结果不完整时报错，不把半成品交给表单', () => {
    expect(() => toLocalBallotResult({ ok: true, ciphertext: 'c' })).toThrow('结果不完整');
  });

  it('成功的结果映射成门户要的形状', () => {
    const result = toLocalBallotResult({
      ok: true,
      ciphertext: 'cipher',
      nullifier: 'null',
      proof: 'proof',
      init_ms: 7000,
      prove_ms: 2000,
    });
    expect(result).toEqual({
      ciphertext: 'cipher',
      nullifier: 'null',
      proof: 'proof',
      initMs: 7000,
      proveMs: 2000,
    });
  });

  it('缺耗时字段时补 0，不让门户崩掉', () => {
    const result = toLocalBallotResult({
      ok: true,
      ciphertext: 'cipher',
      nullifier: 'null',
      proof: 'proof',
    });
    expect(result.initMs).toBe(0);
    expect(result.proveMs).toBe(0);
  });
});

describe('静态资源地址', () => {
  it('相对页面地址解析，门户部署在子路径下也能取到', () => {
    expect(assetUrl('ballot-prover.wasm', 'https://example.test/app/')).toBe(
      'https://example.test/app/ballot-prover.wasm',
    );
    expect(assetUrl('wasm_exec.js', 'https://example.test/app/index.html')).toBe(
      'https://example.test/app/wasm_exec.js',
    );
  });
});

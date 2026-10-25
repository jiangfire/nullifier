import { describe, expect, it } from 'vitest';
import {
  bn254ScalarModulus,
  deriveNullifier,
  deriveProposalField,
  encodeScalarHex,
  generateNullifier,
  generateNullifierSecret,
  nullifierByteSize,
  parseScalarHex,
} from './nullifier';

function fixedBytes(...values: number[]): (length: number) => Uint8Array {
  return (length: number) => {
    const bytes = new Uint8Array(length);
    bytes.set(values.slice(0, length));
    return bytes;
  };
}

describe('generateNullifier', () => {
  it('返回 64 位十六进制字符串', () => {
    const nullifier = generateNullifier(fixedBytes(1, 2, 3));
    expect(nullifier).toHaveLength(nullifierByteSize * 2);
    expect(nullifier).toMatch(/^[0-9a-f]{64}$/);
  });

  it('把超出模数的随机值归约到域内', () => {
    const allBits = new Uint8Array(32).fill(0xff);
    const nullifier = generateNullifier(() => allBits);
    const value = BigInt(`0x${nullifier}`);
    expect(value).toBeLessThan(bn254ScalarModulus);
  });

  it('保留前导零', () => {
    const nullifier = generateNullifier(fixedBytes(0));
    expect(nullifier).toBe('0'.repeat(64));
  });

  it('默认随机源也能产出合法 nullifier', () => {
    const nullifier = generateNullifier();
    expect(nullifier).toMatch(/^[0-9a-f]{64}$/);
    expect(BigInt(`0x${nullifier}`)).toBeLessThan(bn254ScalarModulus);
  });
});

describe('deriveProposalField', () => {
  it('与 Go 侧的 DeriveProposalField 一致', () => {
    expect(encodeScalarHex(deriveProposalField('proposal-1'))).toBe(
      '22e971ef187286f3238ccf7f6552a1605434b5fc3684ef3b642cf011166b253f',
    );
    expect(encodeScalarHex(deriveProposalField('proposal-2'))).toBe(
      '21e6dfa5b112b82f7a3e614744ccf2ce8335cf8bb9979832a17683cb3a96fafd',
    );
  });

  it('不同提案得到不同的域元素', () => {
    expect(deriveProposalField('proposal-1')).not.toBe(deriveProposalField('proposal-2'));
  });

  it('结果落在域内', () => {
    expect(deriveProposalField('proposal-1')).toBeLessThan(bn254ScalarModulus);
  });
});

describe('deriveNullifier', () => {
  it('与 Go 侧的 DeriveNullifier 一致', () => {
    const proposalField = deriveProposalField('proposal-1');
    expect(encodeScalarHex(deriveNullifier(1n, proposalField))).toBe(
      '233e35c4c857f6cb21a2942fcd0d69212e5fd917b38270a3798ae6a70f97065c',
    );
    expect(encodeScalarHex(deriveNullifier(2n, proposalField))).toBe(
      '15777d64eefe8d60ab7b54adef75f9b26ce9630546484a839553dfb5d5415021',
    );
    expect(encodeScalarHex(deriveNullifier(0x9fn, deriveProposalField('proposal-2')))).toBe(
      '11c2a33748f724a0ea65b1c585881fb1d354dccb1cc3e6b69e1d37bc98c8f372',
    );
  });

  it('换秘密会换 nullifier', () => {
    const proposalField = deriveProposalField('proposal-1');
    expect(deriveNullifier(1n, proposalField)).not.toBe(deriveNullifier(2n, proposalField));
  });

  it('换提案会换 nullifier', () => {
    expect(deriveNullifier(1n, deriveProposalField('proposal-1'))).not.toBe(
      deriveNullifier(1n, deriveProposalField('proposal-2')),
    );
  });
});

describe('generateNullifierSecret', () => {
  it('落在域内且可规范编码往返', () => {
    const secret = generateNullifierSecret(fixedBytes(1, 2, 3));
    expect(secret).toBeLessThan(bn254ScalarModulus);
    expect(parseScalarHex(encodeScalarHex(secret))).toBe(secret);
  });

  it('超出模数的采样值会被重采样', () => {
    const outOfRange = new Uint8Array(32).fill(0xff);
    const inRange = new Uint8Array(32);
    inRange[31] = 1;
    let calls = 0;
    const secret = generateNullifierSecret(() => {
      calls += 1;
      return calls === 1 ? outOfRange : inRange;
    });
    expect(calls).toBe(2);
    expect(secret).toBe(1n);
  });
});

describe('标量编码', () => {
  it('拒绝非规范编码', () => {
    expect(() => parseScalarHex(bn254ScalarModulus.toString(16))).toThrow();
    expect(() => parseScalarHex('00')).toThrow();
    expect(() => parseScalarHex('zz'.repeat(32))).toThrow();
  });

  it('编码固定 64 位十六进制', () => {
    expect(encodeScalarHex(1n)).toBe(`${'0'.repeat(63)}1`);
  });
});

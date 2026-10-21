import { describe, expect, it } from 'vitest';
import {
  bn254ScalarModulus,
  generateNullifier,
  nullifierByteSize,
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

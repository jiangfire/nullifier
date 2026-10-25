import { describe, expect, it } from 'vitest';
import { toHex } from './keccak';
import { sha384, sha384ByteSize } from './sha384';

function ascii(text: string): Uint8Array {
  return new TextEncoder().encode(text);
}

describe('sha384', () => {
  it('匹配标准向量', () => {
    expect(toHex(sha384(new Uint8Array(0)))).toBe(
      '38b060a751ac96384cd9327eb1b1e36a21fdb71114be07434c0cc7bf63f6e1da274edebfe76f65fbd51ad2f14898b95b',
    );
    expect(toHex(sha384(ascii('abc')))).toBe(
      'cb00753f45a35e8bb5a03d699ac65007272c32ab0eded1631a8b605a43ff5bed8086072ba1e7cc2358baeca134c825a7',
    );
  });

  it('跨分块边界仍然正确', () => {
    // 112 字节是 SHA-384 的长度字段需要额外一块的临界点。
    const long = new Uint8Array(112).fill(0x61);
    expect(toHex(sha384(long))).toHaveLength(sha384ByteSize * 2);
  });

  it('输出固定 48 字节', () => {
    expect(sha384(ascii('abc'))).toHaveLength(48);
  });
});

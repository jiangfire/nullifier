import { describe, expect, it } from 'vitest';
import { toHex } from './keccak';
import { sha256 } from './sha256';

function ascii(text: string): Uint8Array {
  return new TextEncoder().encode(text);
}

function repeated(char: string, length: number): Uint8Array {
  return ascii(char.repeat(length));
}

describe('sha256', () => {
  it('匹配标准向量', () => {
    expect(toHex(sha256(new Uint8Array(0)))).toBe(
      'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855',
    );
    expect(toHex(sha256(ascii('abc')))).toBe(
      'ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad',
    );
  });

  it('跨分块边界仍然正确', () => {
    // 55/56 是长度字段需要额外一块的临界点，64 是正好一块。
    expect(toHex(sha256(repeated('a', 55)))).toBe(
      '9f4390f8d30c2dd92ec9f095b65e2b9ae9b0a925a5258e241c9f1e910f734318',
    );
    expect(toHex(sha256(repeated('a', 56)))).toBe(
      'b35439a4ac6f0948b6d6f9e3c6af0f5f590ce20f1bde7090ef7970686ec6738a',
    );
    expect(toHex(sha256(repeated('a', 64)))).toBe(
      'ffe054fe7ae0cb6dc65c3af9b61d5209f439851db43d0ba5997337df154668eb',
    );
  });

  it('长输入的摘要与 Go 侧一致', () => {
    expect(toHex(sha256(repeated('a', 1000)))).toBe(
      '41edece42d63e8d9bf515a9ba6932e1c20cbc9f5a5d134645adb5db1b9737ea3',
    );
  });

  it('输出固定 32 字节', () => {
    expect(sha256(ascii('abc'))).toHaveLength(32);
  });
});

import { describe, expect, it } from 'vitest';
import { keccak256, toHex } from './keccak';

function ascii(text: string): Uint8Array {
  return new TextEncoder().encode(text);
}

function repeated(char: string, length: number): Uint8Array {
  return ascii(char.repeat(length));
}

describe('keccak256', () => {
  it('匹配标准向量', () => {
    expect(toHex(keccak256(new Uint8Array(0)))).toBe(
      'c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a470',
    );
    expect(toHex(keccak256(ascii('abc')))).toBe(
      '4e03657aea45a94fc7d47ba826c8d667c0d1e6e33a64a036ec44f58fa12d6c45',
    );
    expect(toHex(keccak256(ascii('seed')))).toBe(
      '66a80b61b29ec044d14c4c8c613e762ba1fb8eeb0c454d1ee00ed6dedaa5b5c5',
    );
  });

  it('跨速率块边界仍然正确', () => {
    // 速率是 136 字节：135/136/137 分别落在填充前、恰好一块、跨到第二块。
    expect(toHex(keccak256(repeated('a', 135)))).toBe(
      '34367dc248bbd832f4e3e69dfaac2f92638bd0bbd18f2912ba4ef454919cf446',
    );
    expect(toHex(keccak256(repeated('a', 136)))).toBe(
      'a6c4d403279fe3e0af03729caada8374b5ca54d8065329a3ebcaeb4b60aa386e',
    );
    expect(toHex(keccak256(repeated('a', 137)))).toBe(
      'd869f639c7046b4929fc92a4d988a8b22c55fbadb802c0c66ebcd484f1915f39',
    );
  });

  it('长输入的摘要与 Go 侧一致', () => {
    expect(toHex(keccak256(repeated('a', 200)))).toBe(
      '96ea54061def936c4be90b518992fdc6f12f535068a256229aca54267b4d084d',
    );
    expect(toHex(keccak256(repeated('a', 1000)))).toBe(
      'b6a4ac1f51884d71f30fa397a5e155de3099e11fc0edef5d08b646e621e19de9',
    );
  });

  it('输出固定 32 字节', () => {
    expect(keccak256(ascii('abc'))).toHaveLength(32);
  });
});

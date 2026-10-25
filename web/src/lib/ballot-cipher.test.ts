import { describe, expect, it } from 'vitest';

import {
  ballotBasePoint,
  ballotCipherSize,
  decodeUncompressedPoint,
  encodePoint,
  encryptBallot,
  isInSubGroup,
  isOnCurve,
} from './ballot-cipher';

/**
 * 跨语言一致性向量。
 *
 * 这些值由 Go 侧 `internal/infrastructure/crypto/ballot` 生成（见
 * `kat_interop_test.go`）：服务端要用同一套点编码解析浏览器提交的密文，
 * 两侧不一致就意味着密文解不开。**改任何一边都必须重新生成并同步。**
 */
const publicKeyUncompressed =
  '2c09976b87cbee617287650964760c30d77b01ab2a38d831b47661270ed3e01e1d' +
  '5a43dc73fe0493cce521cc92a4d34d4837214ce47871c587c567d2d0c72c8f';
const publicKeyCompressed =
  '8f2cc7d0d267c587c57178e44c2137484dd3a492cc21e5cc9304fe73dc435a9d';
const basePointCompressed =
  '8b7d2d877a253c4b7733e1b91f05e0fcedf96bd11c2e572549b2a0f703727925';
const expectedCiphertext =
  '03e8f4c819f576c6516b8d79ae25528f86d68d8250e7ada580022fd42bab255' +
  'ea41c0390e198a87181b9419ecaf472b9c908de35d79bdb085d5f154033efffeb84';
const fixedNonce =
  '000000000000000000000000000000000000000000000000000000003ade68b1';

function hexToBytes(hex: string): Uint8Array {
  const bytes = new Uint8Array(hex.length / 2);
  for (let i = 0; i < bytes.length; i += 1) {
    bytes[i] = Number.parseInt(hex.slice(i * 2, i * 2 + 2), 16);
  }
  return bytes;
}

function bytesToHex(bytes: Uint8Array): string {
  return Array.from(bytes)
    .map((byte) => byte.toString(16).padStart(2, '0'))
    .join('');
}

/** 固定随机源：让加密结果可复现。 */
function fixedRandom(hex: string): (length: number) => Uint8Array {
  return (length: number) => {
    const bytes = hexToBytes(hex);
    if (bytes.length !== length) {
      throw new Error(`fixed random source expects ${length} bytes`);
    }
    return bytes;
  };
}

describe('ballot-cipher', () => {
  it('编码出的基点与 Go 侧一致', () => {
    expect(bytesToHex(encodePoint(ballotBasePoint))).toBe(basePointCompressed);
  });

  it('基点在曲线上且落在素数阶子群里', () => {
    expect(isOnCurve(ballotBasePoint)).toBe(true);
    expect(isInSubGroup(ballotBasePoint)).toBe(true);
  });

  it('加密结果与 Go 侧逐字节一致', () => {
    const ciphertext = encryptBallot(
      hexToBytes(publicKeyUncompressed),
      2,
      fixedRandom(fixedNonce),
    );
    expect(bytesToHex(ciphertext)).toBe(expectedCiphertext);
    expect(ciphertext.length).toBe(ballotCipherSize);
  });

  it('解析未压缩计票公钥', () => {
    const key = decodeUncompressedPoint(hexToBytes(publicKeyUncompressed));
    expect(isOnCurve(key)).toBe(true);
    expect(isInSubGroup(key)).toBe(true);
    // 压缩编码必须与 Go 侧一致。
    expect(bytesToHex(encodePoint(key))).toBe(publicKeyCompressed);
  });

  it('拒绝不在曲线上的计票公钥', () => {
    const broken = hexToBytes(publicKeyUncompressed);
    broken[63] ^= 0x01;
    expect(() =>
      encryptBallot(broken, 0, fixedRandom(fixedNonce)),
    ).toThrow(/not on the curve/);
  });

  it('拒绝越界的选项下标', () => {
    expect(() =>
      encryptBallot(
        hexToBytes(publicKeyUncompressed),
        256,
        fixedRandom(fixedNonce),
      ),
    ).toThrow(/out of range/);
    expect(() =>
      encryptBallot(hexToBytes(publicKeyUncompressed), -1, fixedRandom(fixedNonce)),
    ).toThrow(/out of range/);
  });

  it('每次加密的随机数不同，密文也不同', () => {
    const key = hexToBytes(publicKeyUncompressed);
    const first = encryptBallot(key, 1);
    const second = encryptBallot(key, 1);
    expect(bytesToHex(first)).not.toBe(bytesToHex(second));
    // 但两者都必须是合法的密文结构。
    expect(first[0]).toBe(3);
    expect(second[0]).toBe(3);
  });
});

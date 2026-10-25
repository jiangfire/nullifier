import { describe, expect, it } from 'vitest';
import {
  base64ToBytes,
  blindCredentialMessage,
  bytesToBase64,
  credentialMessage,
  finalizeCredentialSignature,
  parseIssuerKey,
  supportedBlindSignatureVariant,
} from './credential';
import { toHex } from './keccak';

/**
 * 跨语言向量：Go 侧（internal/infrastructure/crypto/blind_rsa_interop_test.go）
 * 用 CIRCL 真库产出同一组值。改任何一边都要重新生成并同步两处。
 *
 * 凭证消息是 `sha256(user_id ‖ 0x00 ‖ user_pub_key)` —— 服务端验签时算的是同一条
 * 消息，对不上就是验签失败。分隔符 0x00 用来固定 user_id 与公钥的边界。
 */
describe('credentialMessage', () => {
  it('与 Go 侧的向量一致', () => {
    const message = credentialMessage(
      'u-blind-1',
      new TextEncoder().encode('one-time-public-key'),
    );
    expect(toHex(message)).toBe(
      'c5518e49bf357e08eace129b9eb4104bb8620264b6c903cdd12a667fefa4e297',
    );
  });

  it('user_id 与公钥的边界由 0x00 固定', () => {
    // 两种切分如果混起来，消息必须不一样 —— 否则 (a, b) 与 (a‖0x00, b) 会撞。
    const first = credentialMessage('u-1', new Uint8Array([1, 2]));
    const second = credentialMessage('u-1', new Uint8Array([0, 1, 2]));
    expect(toHex(first)).not.toBe(toHex(second));
  });

  it('同样的输入得到同样的消息', () => {
    const key = new Uint8Array([9, 8, 7]);
    expect(toHex(credentialMessage('u-1', key))).toBe(
      toHex(credentialMessage('u-1', key)),
    );
  });
});

describe('base64 互转', () => {
  it('往返一致', () => {
    const bytes = new Uint8Array([0, 1, 2, 250, 251, 255]);
    expect(base64ToBytes(bytesToBase64(bytes))).toEqual(bytes);
  });

  it('空数组也是合法的', () => {
    expect(bytesToBase64(new Uint8Array(0))).toBe('');
    expect(base64ToBytes('')).toEqual(new Uint8Array(0));
  });
});

/**
 * 签发方公钥（2048 位 RSA）。
 *
 * 与 Go 侧跨语言向量测试用的是同一把测试密钥 —— 它只出现在测试里，不参与任何
 * 生产路径。
 */
const issuerModulusHex =
  'a8f6a0ae71df41acaa192da461e1e12503dd57e23646fd22fa6130e7fb821895f' +
  '1508a7bd20d5e173736552a9b2e1d2758d97777eaa7350e668239574fb01126a5' +
  '06eeb4773451ef64e948b1b0f5f6365c20e037dc5ab824e32a425777cfa8e4205e' +
  '9be1684fe103d13c628266cf41205d8d849ac977f2f0f400fdb2547c3803e03f5' +
  'e512b32275b8079db69250633dd10c8e3dd77e8246f550bd236a4fad1535dbe06' +
  '17c5b7bc24c1cdb94b2f865e77eee33f03fe593709555f1c1969cc555da085f53' +
  'e267c058aac495c180e2583974dee64d5f9094c025504d4545e5ba65a52c7dbf5b' +
  'ccac52c05244d8f09b9196590e80ad0a0ecfdf0cf2be35325a745a9';

function hexToBytes(hex: string): Uint8Array {
  const bytes = new Uint8Array(hex.length / 2);
  for (let index = 0; index < bytes.length; index += 1) {
    bytes[index] = Number.parseInt(hex.slice(index * 2, index * 2 + 2), 16);
  }
  return bytes;
}

function reservationPayload() {
  return {
    issuer_key_version: 1,
    variant: supportedBlindSignatureVariant,
    modulus: bytesToBase64(hexToBytes(issuerModulusHex)),
    exponent: 65537,
  };
}

describe('parseIssuerKey', () => {
  it('变体名与服务端线路上的名字一致', () => {
    // 服务端发的是 CIRCL `blindrsa.SHA384PSSDeterministic.String()` 的线路名，
    // 不是 Go 侧的枚举标识符。写错时客户端会直接拒签，浏览器端表现为
    // 「领取失败：签发方公钥不可用」。
    // 跨语言锁定：`internal/infrastructure/crypto/blind_rsa_interop_test.go`
    // 的 `TestKAT_BlindRsaVariantName` 断言同一个字符串。
    expect(supportedBlindSignatureVariant).toBe('RSABSSA-SHA384-PSS-Deterministic');
  });

  it('解析出公钥与版本', () => {
    const key = parseIssuerKey(reservationPayload());
    expect(key.version).toBe(1);
    expect(key.variant).toBe(supportedBlindSignatureVariant);
    expect(key.publicKey.exponent).toBe(65537n);
    expect(key.publicKey.modulus.toString(16)).toBe(issuerModulusHex);
  });

  it('变体不认识时直接报错', () => {
    expect(() =>
      parseIssuerKey({ ...reservationPayload(), variant: 'SHA256PSS' }),
    ).toThrow(/unsupported blind signature variant/);
  });

  it('缺字段时报错', () => {
    expect(() => parseIssuerKey({ ...reservationPayload(), exponent: 0 })).toThrow(
      /incomplete/,
    );
    expect(() =>
      parseIssuerKey({ ...reservationPayload(), issuer_key_version: 0 }),
    ).toThrow(/key version/);
  });
});

describe('盲化与解盲', () => {
  it('盲化消息与原文不同，签发方看不出被签内容', () => {
    const issuerKey = parseIssuerKey(reservationPayload());
    const userPubKey = new Uint8Array([1, 2, 3, 4]);
    const blinded = blindCredentialMessage(
      issuerKey,
      'pseudonym-from-pool',
      userPubKey,
    );

    const message = credentialMessage('pseudonym-from-pool', userPubKey);
    expect(toHex(blinded.blindedMessage)).not.toBe(toHex(message));
    expect(blinded.blindedMessage.length).toBe(256);
  });

  it('解盲后拿到与密钥等长的签名', () => {
    const issuerKey = parseIssuerKey(reservationPayload());
    const blinded = blindCredentialMessage(
      issuerKey,
      'pseudonym-from-pool',
      new Uint8Array([1, 2, 3, 4]),
    );

    // 盲签名必须是密钥那么长 —— 服务端拿私钥对盲化消息做一次运算的产物。
    const signature = finalizeCredentialSignature(
      blinded,
      new Uint8Array(256),
      issuerKey,
    );
    expect(signature.length).toBe(256);
  });

  it('盲签名长度不对时报错', () => {
    const issuerKey = parseIssuerKey(reservationPayload());
    const blinded = blindCredentialMessage(
      issuerKey,
      'pseudonym-from-pool',
      new Uint8Array([1, 2, 3, 4]),
    );
    expect(() => finalizeCredentialSignature(blinded, new Uint8Array(10), issuerKey)).toThrow(
      /must be 256 bytes/,
    );
  });
});

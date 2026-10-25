import { describe, expect, it } from 'vitest';
import {
  blindMessage,
  finalizeSignature,
  prepareMessage,
  rsaPublicKeyFromHex,
  verifyBlindSignature,
} from './blind-rsa';
import { toHex } from './keccak';
import { sha256 } from './sha256';

/**
 * 跨语言向量：Go 侧（internal/infrastructure/crypto/blind_rsa_interop_test.go）
 * 用 CIRCL 真库产出同一组值。改任何一边都要重新生成并同步两处。
 */
const modulusHex =
  'a8f6a0ae71df41acaa192da461e1e12503dd57e23646fd22fa6130e7fb821895f' +
  '1508a7bd20d5e173736552a9b2e1d2758d97777eaa7350e668239574fb01126a5' +
  '06eeb4773451ef64e948b1b0f5f6365c20e037dc5ab824e32a425777cfa8e4205e' +
  '9be1684fe103d13c628266cf41205d8d849ac977f2f0f400fdb2547c3803e03f5' +
  'e512b32275b8079db69250633dd10c8e3dd77e8246f550bd236a4fad1535dbe06' +
  '17c5b7bc24c1cdb94b2f865e77eee33f03fe593709555f1c1969cc555da085f53' +
  'e267c058aac495c180e2583974dee64d5f9094c025504d4545e5ba65a52c7dbf5b' +
  'ccac52c05244d8f09b9196590e80ad0a0ecfdf0cf2be35325a745a9';

const blindedMessageHex =
  '63a28a939dd013af69c53a512b8d3dcb45de1da623d1ba587a8ee4a781f6a96f' +
  '01b0ca8e99ddfc6a231b28bc635f3c8f290c472be0a34d004d2f341250673eb92d' +
  '94afddb44ea7c42d2e8d7507df8250b9e8892f6eba2098923bb6b675e875cda01' +
  '2feb6f6cfe980b3af482083c261efcf214c8249230581f9759087268ec89bda61' +
  'c85feafadd09c443aed18d567e79f8723a048358b1ab020a791ebe49e925b66337' +
  '2a95ed4b4faf32d9dcae67b56b11f42a03410884688f7f44fcf9b535a52b8ebf' +
  '6b2fdfd99b47b54de4738169b3a787928ea4c8a309bf73fbdc98830e1c5989d02' +
  '0c4141d083bba7b963db5fad166ec7d5fde38a790e8470583712c2075';

const blindSignatureHex =
  '7f01d9c866f6fac88a8ddbdf415509ed39c5d331f72ef615afc5d156b7349045' +
  'ec68e14138e80ad755099e7994176e6e825edfae7c5a78584f177156efb163465' +
  'e519de4cb24eeda42c5ef51e3e47a19e717b030f75e2f1d6742d7f01e2d250269d' +
  '33ec41d64306083f8a1f1d33d04d0b31a772cef9b36937797cd4c71e9b50ee97' +
  'da97b3faca81d830f725a595b94d93abbe5506ea45c28f81ffd78fa0022baa20e' +
  '5073834e81351563384252d5b13ee9df0be15b58987758412df23b857bd01c83a' +
  '626603d0881310a8325b73c6a2ec2d82c41c5af317501fdcb53ccae5d7feeca3c9' +
  '60ca7b09bcc3d1314566ac8df3a285776267f1cc1f852dd78cde3b337';

const signatureHex =
  '72b5aabb439a72e53ceb7d321ef452cb53ba9c9345fa7fdd2dadfa9637ded064e' +
  'c67626c903d00a23ac91a75770c656919069a4e150060a906300eed61fbe7232b' +
  '6a7349537e26460165460757615b5a0b5d69629924e70ef982e8c348ad25e4d8f' +
  'f8b7e69a58f737957bce4c377ea1ad262c1fd1cdaf4aa8b4e835649c6c8b67932' +
  '3e93cc962409fedc96383d185642744e7bc689a48fe5247ff4c33df5ead8a60d0' +
  'cbc891fa0ca9474d565a30e7c1408316254b7a46f22fd5f390df747f4c6bc0c76' +
  '047569b703e08741c42f2b9da1510033507ee8dff3fda99e1cb993796c2761081' +
  '1eb6299f0955e4ff1ca9f79fb37ec044e8be489816b085cc5787f43d5';

const publicKey = rsaPublicKeyFromHex(modulusHex, '10001');

/** 凭证消息：与 Go 侧 buildCredentialMessage 同口径。 */
function credentialMessage(userId: string, userPubKey: string): Uint8Array {
  const id = new TextEncoder().encode(userId);
  const key = new TextEncoder().encode(userPubKey);
  const input = new Uint8Array(id.length + 1 + key.length);
  input.set(id);
  input[id.length] = 0;
  input.set(key, id.length + 1);
  return sha256(input);
}

/** 固定盐（48 字节）与固定盲化因子，与 Go 侧同口径。 */
function fixedSalt(): Uint8Array {
  const salt = new Uint8Array(48);
  for (let i = 0; i < salt.length; i += 1) {
    salt[i] = i + 1;
  }
  return salt;
}

function fixedBlindingFactor(): bigint {
  const bytes = new Uint8Array(256);
  for (let i = 0; i < bytes.length; i += 1) {
    bytes[i] = (i * 7 + 3) % 251;
  }
  bytes[0] = 0x01;
  bytes[255] |= 1;
  let value = 0n;
  for (const byte of bytes) {
    value = (value << 8n) | BigInt(byte);
  }
  return value;
}

describe('blindMessage', () => {
  it('盲化消息与 Go 侧 CIRCL 逐字节一致', () => {
    const message = credentialMessage('u-blind-1', 'one-time-public-key');
    expect(toHex(message)).toBe(
      'c5518e49bf357e08eace129b9eb4104bb8620264b6c903cdd12a667fefa4e297',
    );

    const { blindedMessage } = blindMessage(
      prepareMessage(message),
      fixedSalt(),
      fixedBlindingFactor(),
      publicKey,
    );
    expect(toHex(blindedMessage)).toBe(blindedMessageHex);
  });

  it('解盲后得到与 Go 侧一致的签名', () => {
    const message = credentialMessage('u-blind-1', 'one-time-public-key');
    const { state } = blindMessage(
      prepareMessage(message),
      fixedSalt(),
      fixedBlindingFactor(),
      publicKey,
    );

    const blindSignature = Uint8Array.from(
      blindSignatureHex.match(/.{2}/g)?.map((byte) => parseInt(byte, 16)) ?? [],
    );
    const signature = finalizeSignature(state, blindSignature, publicKey);
    expect(toHex(signature)).toBe(signatureHex);
    expect(verifyBlindSignature(state, signature, publicKey)).toBe(true);
  });

  it('换消息会换盲化消息', () => {
    const first = blindMessage(
      prepareMessage(credentialMessage('u-1', 'key-1')),
      fixedSalt(),
      fixedBlindingFactor(),
      publicKey,
    );
    const second = blindMessage(
      prepareMessage(credentialMessage('u-2', 'key-2')),
      fixedSalt(),
      fixedBlindingFactor(),
      publicKey,
    );
    expect(toHex(first.blindedMessage)).not.toBe(toHex(second.blindedMessage));
  });

  it('盐长度不对时拒绝', () => {
    expect(() =>
      blindMessage(prepareMessage(new Uint8Array(1)), new Uint8Array(16), 3n, publicKey),
    ).toThrow();
  });
});

describe('prepareMessage', () => {
  it('Deterministic 变体没有随机前缀，原样返回', () => {
    const message = new Uint8Array([1, 2, 3]);
    expect(toHex(prepareMessage(message))).toBe('010203');
  });
});

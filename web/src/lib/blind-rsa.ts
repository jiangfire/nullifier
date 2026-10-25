/**
 * RFC 9474 盲签名的**客户端侧**（RSABSSA-PSS-Deterministic，SHA-384）。
 *
 * 这份实现必须与服务端 CIRCL 的 `blindsign/blindrsa` **逐字节一致**：服务端只
 * 对盲化后的消息做一次 RSA 私钥运算，客户端负责盲化与解盲。两边对同一条消息
 * 算出的盲化消息必须一模一样，否则解出来的签名验不过。
 *
 * 关键事实（读 CIRCL 源码确认）：client 侧的 `Prepare` / `Blind` / `Finalize`
 * **只需要签发方的 RSA 公钥**，不需要私钥，也不需要服务端配合。服务端那一侧
 * 只有 `Signer.BlindSign(blindedMessage)` —— 也就是「签发方全程只见盲化消息」。
 *
 * 一致性由一组跨语言已知答案向量保证：`internal/infrastructure/crypto/blind_rsa_interop_test.go`
 * （Go 侧用 CIRCL 真库产出向量）与 `web/src/lib/blind-rsa.test.ts`（TS 侧断言
 * 同一组值）。改任何一边都要重新生成并同步这两处。
 */

import { sha384, sha384ByteSize } from './sha384';

/** RSA 公钥。 */
export interface RsaPublicKey {
  /** 模数 n。 */
  modulus: bigint;
  /** 公开指数 e。 */
  exponent: bigint;
}

/** 盲签名过程中要留给自己的状态：解盲时要用。 */
export interface BlindSignatureState {
  /** EMSA-PSS 编码后的消息（解盲后要拿它验签）。 */
  encodedMessage: Uint8Array;
  /** 盲化因子的模逆。 */
  blindingFactorInverse: bigint;
}

/** 盲化结果。 */
export interface BlindSignature {
  /** 盲化后的消息，交给签发方去盲签。 */
  blindedMessage: Uint8Array;
  /** 客户端自己留着的解盲状态。**不要交给签发方。 */
  state: BlindSignatureState;
}

/** 模数的字节长度（kLen = ceil(bitLength(n) / 8)）。 */
export function rsaKeySize(publicKey: RsaPublicKey): number {
  return (publicKey.modulus.toString(2).length + 7) >> 3;
}

/** 从十六进制构造公钥（服务端公开公钥时用这个格式）。 */
export function rsaPublicKeyFromHex(modulusHex: string, exponentHex: string): RsaPublicKey {
  const modulus = BigInt(`0x${modulusHex.trim()}`);
  const exponent = BigInt(`0x${exponentHex.trim()}`);
  if (modulus <= 0n || exponent <= 0n) {
    throw new Error('invalid rsa public key');
  }
  return { modulus, exponent };
}

/** 公钥的十六进制形式（`n` 与 `e`），用于日志与调试。 */
export function rsaPublicKeyToHex(publicKey: RsaPublicKey): { modulus: string; exponent: string } {
  return {
    modulus: publicKey.modulus.toString(16),
    exponent: publicKey.exponent.toString(16),
  };
}

function bytesToBigInt(bytes: Uint8Array): bigint {
  let value = 0n;
  for (const byte of bytes) {
    value = (value << 8n) | BigInt(byte);
  }
  return value;
}

function bigIntToBytes(value: bigint, size: number): Uint8Array {
  const bytes = new Uint8Array(size);
  let remaining = value;
  for (let i = size - 1; i >= 0; i -= 1) {
    bytes[i] = Number(remaining & 0xffn);
    remaining >>= 8n;
  }
  return bytes;
}

function mod(value: bigint, modulus: bigint): bigint {
  const reduced = value % modulus;
  return reduced < 0n ? reduced + modulus : reduced;
}

/** 模幂：base^exponent mod modulus。 */
function modPow(base: bigint, exponent: bigint, modulus: bigint): bigint {
  if (modulus <= 0n) {
    throw new Error('invalid modulus');
  }
  let result = 1n;
  let factor = mod(base, modulus);
  let remaining = exponent;
  while (remaining > 0n) {
    if (remaining & 1n) {
      result = (result * factor) % modulus;
    }
    factor = (factor * factor) % modulus;
    remaining >>= 1n;
  }
  return result;
}

/** 模逆（扩展欧几里得）：a 的模 n 逆元。 */
function modInverse(a: bigint, modulus: bigint): bigint {
  let [oldR, r] = [mod(a, modulus), modulus];
  let [oldS, s] = [1n, 0n];
  while (r !== 0n) {
    const quotient = oldR / r;
    [oldR, r] = [r, oldR - quotient * r];
    [oldS, s] = [s, oldS - quotient * s];
  }
  if (oldR !== 1n) {
    throw new Error('value is not invertible');
  }
  return mod(oldS, modulus);
}

/** MGF1（RFC 8017 附录 B.2.1），掩码生成函数。 */
function mgf1(seed: Uint8Array, length: number): Uint8Array {
  const output = new Uint8Array(length);
  let offset = 0;
  let counter = 0;
  while (offset < length) {
    const block = new Uint8Array(seed.length + 4);
    block.set(seed);
    block[seed.length] = (counter >>> 24) & 0xff;
    block[seed.length + 1] = (counter >>> 16) & 0xff;
    block[seed.length + 2] = (counter >>> 8) & 0xff;
    block[seed.length + 3] = counter & 0xff;
    const digest = sha384(block);
    const take = Math.min(digest.length, length - offset);
    output.set(digest.subarray(0, take), offset);
    offset += take;
    counter += 1;
  }
  return output;
}

/**
 * EMSA-PSS 编码（RFC 8017 §9.1.1）。
 *
 * 与 CIRCL 的 `emsaPSSEncode` 同口径：EM = maskedDB || H || 0xbc，
 * DB = PS || 0x01 || salt，maskedDB = DB xor MGF1(H)。
 */
function emsaPssEncode(
  messageHash: Uint8Array,
  emBits: number,
  salt: Uint8Array,
): Uint8Array {
  const hLen = messageHash.length;
  const sLen = salt.length;
  const emLen = (emBits + 7) >> 3;
  if (emLen < hLen + sLen + 2) {
    throw new Error('rsa key size too small for pss signature');
  }
  const psLen = emLen - sLen - hLen - 2;

  // DB = PS || 0x01 || salt
  const db = new Uint8Array(psLen + 1 + sLen);
  db[psLen] = 0x01;
  db.set(salt, psLen + 1);

  // H = Hash(0x00 * 8 || mHash || salt)
  const prefix = new Uint8Array(8);
  const mPrime = new Uint8Array(8 + hLen + sLen);
  mPrime.set(prefix);
  mPrime.set(messageHash, 8);
  mPrime.set(salt, 8 + hLen);
  const h = sha384(mPrime);

  // maskedDB = DB xor MGF1(H)
  const dbMask = mgf1(h, emLen - hLen - 1);
  const maskedDB = new Uint8Array(db.length);
  for (let i = 0; i < db.length; i += 1) {
    maskedDB[i] = db[i] ^ dbMask[i];
  }
  // 左边 8*emLen - emBits 个比特清零。
  maskedDB[0] &= 0xff >> (8 * emLen - emBits);

  const em = new Uint8Array(emLen);
  em.set(maskedDB, 0);
  em.set(h, maskedDB.length);
  em[emLen - 1] = 0xbc;
  return em;
}

/**
 * 准备消息（RFC 9474 的 Prepare）。
 *
 * CIRCL 的 `SHA384PSSDeterministic` 变体没有随机前缀，所以原样返回。
 */
export function prepareMessage(message: Uint8Array): Uint8Array {
  return message.slice();
}

/**
 * 盲化消息：把消息变成签发方看不出内容的盲化消息。
 *
 * @param preparedMessage 准备好的消息
 * @param salt PSS 盐（48 字节，与 SHA-384 等长）
 * @param blindingFactor 盲化因子 r（与模数互素）
 * @param publicKey 签发方公钥
 */
export function blindMessage(
  preparedMessage: Uint8Array,
  salt: Uint8Array,
  blindingFactor: bigint,
  publicKey: RsaPublicKey,
): BlindSignature {
  if (salt.length !== sha384ByteSize) {
    throw new Error(`pss salt must be ${sha384ByteSize} bytes`);
  }
  const keySize = rsaKeySize(publicKey);
  const emBits = publicKey.modulus.toString(2).length - 1;
  const encodedMessage = emsaPssEncode(sha384(preparedMessage), emBits, salt);

  const message = bytesToBigInt(encodedMessage);
  const masked = modPow(blindingFactor, publicKey.exponent, publicKey.modulus);
  const blinded = (message * masked) % publicKey.modulus;

  return {
    blindedMessage: bigIntToBytes(blinded, keySize),
    state: {
      encodedMessage,
      blindingFactorInverse: modInverse(blindingFactor, publicKey.modulus),
    },
  };
}

/**
 * 解盲：把盲签名还原成最终签名。
 *
 * @param state 盲化时留下的状态
 * @param blindSignature 签发方返回的盲签名
 * @param publicKey 签发方公钥
 */
export function finalizeSignature(
  state: BlindSignatureState,
  blindSignature: Uint8Array,
  publicKey: RsaPublicKey,
): Uint8Array {
  const keySize = rsaKeySize(publicKey);
  if (blindSignature.length !== keySize) {
    throw new Error(`blind signature must be ${keySize} bytes`);
  }
  const blinded = bytesToBigInt(blindSignature);
  const signature = (blinded * state.blindingFactorInverse) % publicKey.modulus;
  return bigIntToBytes(signature, keySize);
}

/**
 * 验签：确认解盲出来的签名确实是对这条消息的签名。
 *
 * 与 CIRCL 的 `VerifyBlindSignature` 同口径：sig^e mod n 必须等于编码后的消息。
 */
export function verifyBlindSignature(
  state: BlindSignatureState,
  signature: Uint8Array,
  publicKey: RsaPublicKey,
): boolean {
  const value = bytesToBigInt(signature);
  const recovered = modPow(value, publicKey.exponent, publicKey.modulus);
  const expected = bytesToBigInt(state.encodedMessage);
  return recovered === expected;
}

/** 生成 PSS 盐（48 字节）。 */
export function generateSalt(
  fillRandom: (length: number) => Uint8Array = defaultFillRandom,
): Uint8Array {
  return fillRandom(sha384ByteSize);
}

/**
 * 采样盲化因子 r。
 *
 * 必须是与模数互素的非零值，否则解不出盲化因子。分布上的轻微偏差对盲化因子
 * 没有影响 —— 它只需要不可预测，不需要均匀。
 */
export function generateBlindingFactor(
  publicKey: RsaPublicKey,
  fillRandom: (length: number) => Uint8Array = defaultFillRandom,
): bigint {
  const keySize = rsaKeySize(publicKey);
  for (;;) {
    const candidate = bytesToBigInt(fillRandom(keySize)) % publicKey.modulus;
    if (candidate === 0n) {
      continue;
    }
    try {
      modInverse(candidate, publicKey.modulus);
      return candidate;
    } catch {
      continue;
    }
  }
}

/** 默认的随机源：浏览器加密随机数。 */
function defaultFillRandom(length: number): Uint8Array {
  const bytes = new Uint8Array(length);
  crypto.getRandomValues(bytes);
  return bytes;
}

/**
 * 凭证盲签名的**客户端侧**。
 *
 * 协议是三步：
 *  1. 预留签发权 —— 服务端给出一个凭证假名（user_id）与签发方公钥，**不签名**
 *  2. 盲签名 —— 客户端本地把凭证消息盲化，把盲化消息发给服务端，服务端回盲签名
 *  3. attest —— 客户端解盲得到最终签名，交回服务端验签并落库
 *
 * 关键：**客户端从头到尾不把公钥发给服务端**（只有第 3 步才发）。服务端一旦
 * 知道公钥就能算出凭证消息的原文，盲化当场失效 —— 它手上有私钥，根本不需要
 * 解盲。所以第 1 步的请求体里不能有公钥。
 *
 * 与 Go 侧的一致性由一组跨语言已知答案向量锁死
 * （`internal/infrastructure/crypto/blind_rsa_interop_test.go` 与
 * `web/src/lib/blind-rsa.test.ts`）。
 */

import {
  blindMessage,
  finalizeSignature,
  generateBlindingFactor,
  generateSalt,
  prepareMessage,
  type BlindSignature,
  type RsaPublicKey,
} from './blind-rsa';
import { sha256 } from './sha256';

/** 当前支持的盲签名变体。服务端换变体时这里要跟着改。 */
export const supportedBlindSignatureVariant = 'SHA384PSSDeterministic';

/** 签发方公钥与密钥版本。 */
export interface IssuerKey {
  /** 密钥版本：盲签名时要告诉服务端用的是哪一版。 */
  version: number;
  /** 变体名。 */
  variant: string;
  /** RSA 公钥。 */
  publicKey: RsaPublicKey;
}

/** base64 解码成字节。 */
export function base64ToBytes(value: string): Uint8Array {
  const binary = atob(value);
  const bytes = new Uint8Array(binary.length);
  for (let index = 0; index < binary.length; index += 1) {
    bytes[index] = binary.charCodeAt(index);
  }
  return bytes;
}

/** 字节编码成 base64。 */
export function bytesToBase64(bytes: Uint8Array): string {
  let output = '';
  for (const value of bytes) {
    output += String.fromCharCode(value);
  }
  return btoa(output);
}

/**
 * 凭证消息：`sha256(user_id ‖ 0x00 ‖ user_pub_key)`。
 *
 * 与 Go 侧 `crypto.CredentialMessage` 同口径 —— 服务端验签时算的是同一条消息，
 * 对不上就是验签失败。分隔符 0x00 用来固定 user_id 与公钥的边界。
 */
export function credentialMessage(userId: string, userPubKey: Uint8Array): Uint8Array {
  const userIdBytes = new TextEncoder().encode(userId);
  const message = new Uint8Array(userIdBytes.length + 1 + userPubKey.length);
  message.set(userIdBytes, 0);
  message[userIdBytes.length] = 0x00;
  message.set(userPubKey, userIdBytes.length + 1);
  return sha256(message);
}

/**
 * 从「预留签发权」的响应里解析签发方公钥。
 *
 * 模数是 base64 的大端字节；这里直接转成 BigInt。
 */
export function parseIssuerKey(payload: Record<string, unknown>): IssuerKey {
  const version = numberOf(payload, 'issuer_key_version');
  const variant = textOf(payload, 'variant');
  const modulus = textOf(payload, 'modulus');
  const exponent = numberOf(payload, 'exponent');
  if (version <= 0) {
    throw new Error('issuer key version is missing');
  }
  if (variant !== supportedBlindSignatureVariant) {
    throw new Error(`unsupported blind signature variant: ${variant || '(empty)'}`);
  }
  if (modulus === '' || exponent <= 0) {
    throw new Error('issuer public key is incomplete');
  }
  const modulusValue = bytesToBigInt(base64ToBytes(modulus));
  if (modulusValue <= 0n) {
    throw new Error('invalid issuer modulus');
  }
  return { version, variant, publicKey: { modulus: modulusValue, exponent: BigInt(exponent) } };
}

/** 盲化凭证消息：把消息变成签发方看不出内容的盲化消息。 */
export function blindCredentialMessage(
  issuerKey: IssuerKey,
  userId: string,
  userPubKey: Uint8Array,
): BlindSignature {
  const message = credentialMessage(userId, userPubKey);
  const prepared = prepareMessage(message);
  const salt = generateSalt();
  const factor = generateBlindingFactor(issuerKey.publicKey);
  return blindMessage(prepared, salt, factor, issuerKey.publicKey);
}

/** 解盲：把签发方返回的盲签名还原成最终签名。 */
export function finalizeCredentialSignature(
  blinded: BlindSignature,
  blindSignature: Uint8Array,
  issuerKey: IssuerKey,
): Uint8Array {
  return finalizeSignature(blinded.state, blindSignature, issuerKey.publicKey);
}

function bytesToBigInt(bytes: Uint8Array): bigint {
  let value = 0n;
  for (const byte of bytes) {
    value = (value << 8n) | BigInt(byte);
  }
  return value;
}

function textOf(payload: Record<string, unknown>, key: string): string {
  const value = payload[key];
  return typeof value === 'string' ? value.trim() : '';
}

function numberOf(payload: Record<string, unknown>, key: string): number {
  const value = payload[key];
  if (typeof value === 'number' && Number.isFinite(value)) {
    return value;
  }
  if (typeof value === 'string' && value.trim() !== '') {
    const parsed = Number(value);
    return Number.isFinite(parsed) ? parsed : 0;
  }
  return 0;
}

/**
 * nullifier 的编码与派生规则。
 *
 * 服务端把 nullifier 当作 BN254 标量域里的元素：它是防重投的公开标识，
 * 必须由投票人的秘密单向推导出来，而且编码规范唯一 —— 同一个值不能有
 * 两种字节表示，否则票表上的唯一索引就拦不住双重投票。
 *
 * 电路对 nullifier 的约束是 `nullifier = MiMC(秘密, 提案域元素)`。这意味着
 * **只有经 `deriveNullifier` 派生出来的 nullifier 才出得了证明**；直接拿
 * 随机域元素当 nullifier（`generateNullifier`），电路那条约束永远过不去。
 */

import { mimcHash } from './mimc';
import { sha256 } from './sha256';
import {
  bn254ScalarModulus,
  bytesToBigInt,
  encodeScalarHex,
  modField,
  parseScalarHex,
  sampleScalar,
  scalarByteSize,
} from './field';

export { bn254ScalarModulus, encodeScalarHex, parseScalarHex };

/** nullifier 的字节长度（域元素的规范编码）。 */
export const nullifierByteSize = scalarByteSize;

/**
 * 生成一个规范编码的 nullifier。
 *
 * 随机字节先按模 r 归约，保证输出一定落在域内 —— 服务端只接受规范编码，
 * 直接把 32 个随机字节丢过去大约有十分之一的概率被拒。
 *
 * ⚠️ **这条路径不满足电路约束**：它产出的是随机域元素，没有绑定任何秘密，
 * 电路里的 `nullifier = MiMC(秘密, 提案域元素)` 无法满足，因此它产出的
 * nullifier 出不了证明。证明路径请用 `deriveNullifier`。
 */
export function generateNullifier(
  fillRandom: (length: number) => Uint8Array = defaultFillRandom,
): string {
  const bytes = fillRandom(nullifierByteSize);
  let value = 0n;
  for (const byte of bytes) {
    value = (value << 8n) | BigInt(byte);
  }
  value %= bn254ScalarModulus;
  return value.toString(16).padStart(nullifierByteSize * 2, '0');
}

/**
 * 把提案 ID 映射成标量域元素。
 *
 * 与 Go 侧的 `DeriveProposalField` 一致：SHA-256 之后再按模 r 归约。提案 ID
 * 是公开值，不需要在电路里复算，电路只要求两侧得到同一个域元素。
 */
export function deriveProposalField(proposalId: string): bigint {
  const digest = sha256(new TextEncoder().encode(proposalId));
  return modField(bytesToBigInt(digest));
}

/**
 * 用 nullifier 秘密与提案域元素推导 nullifier。
 *
 * 与 Go 侧的 `DeriveNullifier` 及电路里的 MiMC 约束逐位一致。秘密必须留在
 * 本地：prover 要用它作为电路的 `NullifierSecret` 见证，而它一旦上传给
 * 服务端，别人就能算出同一个 nullifier 抢先投票。
 */
export function deriveNullifier(secret: bigint, proposalField: bigint): bigint {
  return mimcHash(secret, proposalField);
}

/**
 * 采样一个 nullifier 秘密。
 *
 * 秘密只留在本地（浏览器里就是内存，CLI 里是选票文件），公开出去的只有
 * 派生出来的 nullifier。
 */
export function generateNullifierSecret(
  fillRandom: (length: number) => Uint8Array = defaultFillRandom,
): bigint {
  return sampleScalar(fillRandom);
}

/** 默认的随机源：浏览器加密随机数。 */
function defaultFillRandom(length: number): Uint8Array {
  const bytes = new Uint8Array(length);
  crypto.getRandomValues(bytes);
  return bytes;
}

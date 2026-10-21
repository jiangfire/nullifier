/**
 * nullifier 的编码规则。
 *
 * 服务端把 nullifier 当作 BN254 标量域里的元素：它是防重投的公开标识，
 * 必须由投票人的秘密单向推导出来，而且编码规范唯一 —— 同一个值不能有
 * 两种字节表示，否则票表上的唯一索引就拦不住双重投票。
 */

/** BN254 标量域的模数。 */
export const bn254ScalarModulus =
  21888242871839275222246405745257275088548364400416034343698204186575808495617n;

/** nullifier 的字节长度（域元素的规范编码）。 */
export const nullifierByteSize = 32;

/**
 * 生成一个规范编码的 nullifier。
 *
 * 随机字节先按模 r 归约，保证输出一定落在域内 —— 服务端只接受规范编码，
 * 直接把 32 个随机字节丢过去大约有十分之一的概率被拒。
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

/** 默认的随机源：浏览器加密随机数。 */
function defaultFillRandom(length: number): Uint8Array {
  const bytes = new Uint8Array(length);
  crypto.getRandomValues(bytes);
  return bytes;
}

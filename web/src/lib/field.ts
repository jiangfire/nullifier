/**
 * BN254 标量域的公共运算与标量编码。
 *
 * 标量域的模数是电路、密文与哈希共用的口径：任何一处写错，跨语言的一致性
 * 就会断掉。域元素的字节表示必须**规范唯一** —— 同一个值不能有两种字节表示，
 * 否则票表上的唯一索引就拦不住用变体写法重复投票。
 */

/** BN254 标量域的模数。 */
export const bn254ScalarModulus =
  21888242871839275222246405745257275088548364400416034343698204186575808495617n;

/** 标量的字节长度。 */
export const scalarByteSize = 32;

/** 模归约：把任意整数折回域内（结果落在 [0, r)）。 */
export function modField(value: bigint): bigint {
  const reduced = value % bn254ScalarModulus;
  return reduced < 0n ? reduced + bn254ScalarModulus : reduced;
}

/** 域加法。 */
export function addField(a: bigint, b: bigint): bigint {
  return modField(a + b);
}

/** 域乘法。 */
export function mulField(a: bigint, b: bigint): bigint {
  return modField(a * b);
}

/** 字节串（大端）转成整数。 */
export function bytesToBigInt(bytes: Uint8Array): bigint {
  let value = 0n;
  for (const byte of bytes) {
    value = (value << 8n) | BigInt(byte);
  }
  return value;
}

/**
 * 在域内均匀采样一个标量。
 *
 * 超出模数的取值直接重采样 —— 按模归约会引入偏差，秘密的分布就不均匀了。
 */
export function sampleScalar(fillRandom: (length: number) => Uint8Array): bigint {
  for (;;) {
    const value = bytesToBigInt(fillRandom(scalarByteSize));
    if (value >= bn254ScalarModulus) {
      continue;
    }
    return value;
  }
}

/**
 * 把标量编码成 64 位十六进制字符串（大端、小写）。
 *
 * 编码必须是规范形式：值必须落在域内，否则同一个标量会有多种写法。
 */
export function encodeScalarHex(value: bigint): string {
  if (value < 0n || value >= bn254ScalarModulus) {
    throw new Error('scalar is out of the field range');
  }
  return value.toString(16).padStart(scalarByteSize * 2, '0');
}

/**
 * 解析十六进制编码的标量。
 *
 * 与「按模归约」不同：非规范编码直接拒绝，避免同一个标量出现多种字节表示。
 */
export function parseScalarHex(value: string): bigint {
  const normalized = value.trim().toLowerCase();
  if (!/^[0-9a-f]*$/.test(normalized) || normalized.length !== scalarByteSize * 2) {
    throw new Error(`invalid scalar encoding: ${value}`);
  }
  const parsed = BigInt(`0x${normalized}`);
  if (parsed >= bn254ScalarModulus) {
    throw new Error('scalar encoding is not canonical');
  }
  return parsed;
}

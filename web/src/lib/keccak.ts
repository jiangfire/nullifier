/**
 * Keccak-256（原始 Keccak 填充，**不是** SHA3-256）。
 *
 * 浏览器端需要它来派生 MiMC 的轮常量：gnark 的 MiMC 常量由
 * 「Keccak-256 链」生成（seed = "seed"），两边必须用同一个哈希函数，
 * 否则轮常量不同、nullifier 就算不出电路认可的值。
 *
 * 注意填充字节是 0x01（legacy Keccak）而不是 SHA3 的 0x06 —— 用错会得到
 * 完全不同的摘要，而且不会报错。
 */

/** 64 位掩码。 */
const MASK64 = (1n << 64n) - 1n;

/** Keccak-f[1600] 的 24 个轮常量。 */
const ROUND_CONSTANTS: readonly bigint[] = [
  0x0000000000000001n,
  0x0000000000008082n,
  0x800000000000808an,
  0x8000000080008000n,
  0x000000000000808bn,
  0x0000000080000001n,
  0x8000000080008081n,
  0x8000000000008009n,
  0x000000000000008an,
  0x0000000000000088n,
  0x0000000080008009n,
  0x000000008000000an,
  0x000000008000808bn,
  0x800000000000008bn,
  0x8000000000008089n,
  0x8000000000008003n,
  0x8000000000008002n,
  0x8000000000000080n,
  0x000000000000800an,
  0x800000008000000an,
  0x8000000080008081n,
  0x8000000000008080n,
  0x0000000080000001n,
  0x8000000080008008n,
];

/** 各 lane 的旋转偏移，按 lane 序号 x + 5*y 排列。 */
const ROTATION_OFFSETS: readonly number[] = [
  0, 1, 62, 28, 27, 36, 44, 6, 55, 20, 3, 10, 43, 25, 39, 41, 45, 15, 21, 8, 18, 2, 61, 56, 14,
];

/** Keccak-256 的速率：1088 比特 = 136 字节。 */
const RATE_BYTES = 136;

/** Keccak-256 的输出长度。 */
const KECCAK256_SIZE = 32;

/** 64 位左循环移位。 */
function rotl(value: bigint, shift: number): bigint {
  const amount = BigInt(shift);
  return ((value << amount) | (value >> (64n - amount))) & MASK64;
}

/** Keccak-f[1600] 置换：就地改写 25 个 lane 的状态。 */
function keccakF1600(state: BigUint64Array): void {
  const c = new BigUint64Array(5);
  const b = new BigUint64Array(25);

  for (let round = 0; round < 24; round += 1) {
    // θ：列奇偶校验与差分
    for (let x = 0; x < 5; x += 1) {
      c[x] = state[x] ^ state[x + 5] ^ state[x + 10] ^ state[x + 15] ^ state[x + 20];
    }
    for (let x = 0; x < 5; x += 1) {
      const d = c[(x + 4) % 5] ^ rotl(c[(x + 1) % 5], 1);
      for (let y = 0; y < 5; y += 1) {
        state[x + 5 * y] ^= d;
      }
    }

    // ρ 与 π：旋转并重排 lane
    for (let x = 0; x < 5; x += 1) {
      for (let y = 0; y < 5; y += 1) {
        const source = x + 5 * y;
        const target = y + 5 * ((2 * x + 3 * y) % 5);
        b[target] = rotl(state[source], ROTATION_OFFSETS[source]);
      }
    }

    // χ：行上的非线性
    for (let y = 0; y < 5; y += 1) {
      for (let x = 0; x < 5; x += 1) {
        state[x + 5 * y] = b[x + 5 * y] ^ (~b[((x + 1) % 5) + 5 * y] & b[((x + 2) % 5) + 5 * y]);
      }
    }

    // ι：轮常量
    state[0] ^= ROUND_CONSTANTS[round];
  }
}

/**
 * 计算 Keccak-256 摘要。
 *
 * @param data 输入字节
 * @returns 32 字节摘要
 */
export function keccak256(data: Uint8Array): Uint8Array {
  // 填充：legacy Keccak 用 0x01 开头、0x80 收尾（SHA3 用的是 0x06）。
  const paddedLength = Math.ceil((data.length + 1) / RATE_BYTES) * RATE_BYTES;
  const padded = new Uint8Array(paddedLength);
  padded.set(data);
  padded[data.length] = 0x01;
  padded[paddedLength - 1] |= 0x80;

  const state = new BigUint64Array(25);
  const view = new DataView(padded.buffer);

  for (let offset = 0; offset < paddedLength; offset += RATE_BYTES) {
    for (let lane = 0; lane < RATE_BYTES / 8; lane += 1) {
      state[lane] ^= view.getBigUint64(offset + lane * 8, true);
    }
    keccakF1600(state);
  }

  const digest = new Uint8Array(KECCAK256_SIZE);
  const digestView = new DataView(digest.buffer);
  for (let i = 0; i < KECCAK256_SIZE / 8; i += 1) {
    digestView.setBigUint64(i * 8, state[i], true);
  }
  return digest;
}

/** 把字节串转成小写十六进制。 */
export function toHex(bytes: Uint8Array): string {
  let hex = '';
  for (const byte of bytes) {
    hex += byte.toString(16).padStart(2, '0');
  }
  return hex;
}

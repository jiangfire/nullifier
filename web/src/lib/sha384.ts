/**
 * SHA-384（纯 TypeScript 实现）。
 *
 * 用途：RFC 9474 盲签名（RSABSSA-PSS-Deterministic）的消息摘要。服务端用
 * CIRCL 的 `blindsign/blindrsa`，变体是 `SHA384PSSDeterministic` —— 两边
 * 必须用同一个哈希，否则编码出的明文不同，签名根本解不开。
 *
 * SHA-384 是 SHA-512 的截断版：同样的压缩函数，初始值不同，输出取前 384 位。
 *
 * 为什么不用 WebCrypto 的 `crypto.subtle.digest`：那要求安全上下文
 * （https 或 localhost），门户被部署在内网 http 地址下会直接不可用 ——
 * 而这是投票路径上的必经环节。
 */

/** 64 位掩码。 */
const MASK64 = (1n << 64n) - 1n;

/** SHA-512 的 80 个轮常量。 */
const ROUND_CONSTANTS: readonly bigint[] = [
  0x428a2f98d728ae22n, 0x7137449123ef65cdn, 0xb5c0fbcfec4d3b2fn, 0xe9b5dba58189dbbcn,
  0x3956c25bf348b538n, 0x59f111f1b605d019n, 0x923f82a4af194f9bn, 0xab1c5ed5da6d8118n,
  0xd807aa98a3030242n, 0x12835b0145706fben, 0x243185be4ee4b28cn, 0x550c7dc3d5ffb4e2n,
  0x72be5d74f27b896fn, 0x80deb1fe3b1696b1n, 0x9bdc06a725c71235n, 0xc19bf174cf692694n,
  0xe49b69c19ef14ad2n, 0xefbe4786384f25e3n, 0x0fc19dc68b8cd5b5n, 0x240ca1cc77ac9c65n,
  0x2de92c6f592b0275n, 0x4a7484aa6ea6e483n, 0x5cb0a9dcbd41fbd4n, 0x76f988da831153b5n,
  0x983e5152ee66dfabn, 0xa831c66d2db43210n, 0xb00327c898fb213fn, 0xbf597fc7beef0ee4n,
  0xc6e00bf33da88fc2n, 0xd5a79147930aa725n, 0x06ca6351e003826fn, 0x142929670a0e6e70n,
  0x27b70a8546d22ffcn, 0x2e1b21385c26c926n, 0x4d2c6dfc5ac42aedn, 0x53380d139d95b3dfn,
  0x650a73548baf63den, 0x766a0abb3c77b2a8n, 0x81c2c92e47edaee6n, 0x92722c851482353bn,
  0xa2bfe8a14cf10364n, 0xa81a664bbc423001n, 0xc24b8b70d0f89791n, 0xc76c51a30654be30n,
  0xd192e819d6ef5218n, 0xd69906245565a910n, 0xf40e35855771202an, 0x106aa07032bbd1b8n,
  0x19a4c116b8d2d0c8n, 0x1e376c085141ab53n, 0x2748774cdf8eeb99n, 0x34b0bcb5e19b48a8n,
  0x391c0cb3c5c95a63n, 0x4ed8aa4ae3418acbn, 0x5b9cca4f7763e373n, 0x682e6ff3d6b2b8a3n,
  0x748f82ee5defb2fcn, 0x78a5636f43172f60n, 0x84c87814a1f0ab72n, 0x8cc702081a6439ecn,
  0x90befffa23631e28n, 0xa4506cebde82bde9n, 0xbef9a3f7b2c67915n, 0xc67178f2e372532bn,
  0xca273eceea26619cn, 0xd186b8c721c0c207n, 0xeada7dd6cde0eb1en, 0xf57d4f7fee6ed178n,
  0x06f067aa72176fban, 0x0a637dc5a2c898a6n, 0x113f9804bef90daen, 0x1b710b35131c471bn,
  0x28db77f523047d84n, 0x32caab7b40c72493n, 0x3c9ebe0a15c9bebcn, 0x431d67c49c100d4cn,
  0x4cc5d4becb3e42b6n, 0x597f299cfc657e2an, 0x5fcb6fab3ad6faecn, 0x6c44198c4a475817n,
];

/** SHA-384 的初始状态。 */
const INITIAL_STATE: readonly bigint[] = [
  0xcbbb9d5dc1059ed8n, 0x629a292a367cd507n, 0x9159015a3070dd17n, 0x152fecd8f70e5939n,
  0x67332667ffc00b31n, 0x8eb44a8768581511n, 0xdb0c2e0d64f98fa7n, 0x47b5481dbefa4fa4n,
];

/** SHA-384 的摘要长度。 */
export const sha384ByteSize = 48;

/** 64 位右循环移位。 */
function rotr(value: bigint, shift: bigint): bigint {
  return ((value >> shift) | (value << (64n - shift))) & MASK64;
}

/**
 * 计算 SHA-384 摘要。
 *
 * @param data 输入字节
 * @returns 48 字节摘要
 */
export function sha384(data: Uint8Array): Uint8Array {
  // 填充：0x80 开头，补零到 112 字节的倍数，再接 128 位大端长度。
  const paddedLength = Math.ceil((data.length + 17) / 128) * 128;
  const padded = new Uint8Array(paddedLength);
  padded.set(data);
  padded[data.length] = 0x80;
  // 长度是比特数，写成 128 位大端：高 64 位为 0，低 64 位在末尾。
  let remaining = BigInt(data.length) * 8n;
  for (let i = paddedLength - 1; i >= paddedLength - 8; i -= 1) {
    padded[i] = Number(remaining & 0xffn);
    remaining >>= 8n;
  }

  const state = INITIAL_STATE.slice();
  const schedule = new Array<bigint>(80);

  for (let offset = 0; offset < paddedLength; offset += 128) {
    for (let i = 0; i < 16; i += 1) {
      let word = 0n;
      for (let j = 0; j < 8; j += 1) {
        word = (word << 8n) | BigInt(padded[offset + i * 8 + j]);
      }
      schedule[i] = word;
    }
    for (let i = 16; i < 80; i += 1) {
      const s0 = rotr(schedule[i - 15], 1n) ^ rotr(schedule[i - 15], 8n) ^ (schedule[i - 15] >> 7n);
      const s1 = rotr(schedule[i - 2], 19n) ^ rotr(schedule[i - 2], 61n) ^ (schedule[i - 2] >> 6n);
      schedule[i] = (schedule[i - 16] + s0 + schedule[i - 7] + s1) & MASK64;
    }

    let a = state[0];
    let b = state[1];
    let c = state[2];
    let d = state[3];
    let e = state[4];
    let f = state[5];
    let g = state[6];
    let h = state[7];

    for (let i = 0; i < 80; i += 1) {
      const sum1 = rotr(e, 14n) ^ rotr(e, 18n) ^ rotr(e, 41n);
      const choose = (e & f) ^ (~e & g);
      const temp1 = (h + sum1 + choose + ROUND_CONSTANTS[i] + schedule[i]) & MASK64;
      const sum0 = rotr(a, 28n) ^ rotr(a, 34n) ^ rotr(a, 39n);
      const majority = (a & b) ^ (a & c) ^ (b & c);
      const temp2 = (sum0 + majority) & MASK64;

      h = g;
      g = f;
      f = e;
      e = (d + temp1) & MASK64;
      d = c;
      c = b;
      b = a;
      a = (temp1 + temp2) & MASK64;
    }

    state[0] = (state[0] + a) & MASK64;
    state[1] = (state[1] + b) & MASK64;
    state[2] = (state[2] + c) & MASK64;
    state[3] = (state[3] + d) & MASK64;
    state[4] = (state[4] + e) & MASK64;
    state[5] = (state[5] + f) & MASK64;
    state[6] = (state[6] + g) & MASK64;
    state[7] = (state[7] + h) & MASK64;
  }

  // 取前 6 个 64 位字 = 384 位。
  const digest = new Uint8Array(sha384ByteSize);
  for (let i = 0; i < 6; i += 1) {
    let word = state[i];
    for (let j = 7; j >= 0; j -= 1) {
      digest[i * 8 + j] = Number(word & 0xffn);
      word >>= 8n;
    }
  }
  return digest;
}

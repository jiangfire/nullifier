/**
 * 选票密文的浏览器端实现（格式 v3）。
 *
 * 选票密文跑在 BN254 的 twisted Edwards 曲线上：选票下标以指数形式加密，
 * 服务端在没有门限私钥的前提下无法还原「投了哪个选项」。
 *
 * 这份实现必须与 Go 侧 `internal/infrastructure/crypto/ballot` **逐字节一致**：
 * 服务端要用同一套点编码解析客户端提交的密文，电路也要在同一条曲线上复算
 * 密文关系。跨语言一致性由 `ballot-cipher.test.ts` 的已知答案向量保证。
 */

/** BN254 标量域的模数。曲线坐标与标量都在这个域里。 */
export const bn254ScalarModulus =
  21888242871839275222246405745257275088548364400416034343698204186575808495617n;

/** twisted Edwards 曲线参数 a（= -1）。 */
const curveA = bn254ScalarModulus - 1n;

/** twisted Edwards 曲线参数 d。 */
const curveD =
  12181644023421730124874158521699555681764249180949974110617291017600649128846n;

/** 素数阶子群的阶（= 标量域阶的 1/8）。 */
const groupOrder =
  2736030358979909402780800718157159386076813972158567259200215660948447373041n;

/** 基点 X 坐标。 */
const baseX =
  9671717474070082183213120605117400219616337014328744928644933853176787189663n;

/** 基点 Y 坐标。 */
const baseY =
  16950150798460657717958625567821834550301663161624707787222815936182638968203n;

/** 压缩点的字节长度。 */
export const ballotPointSize = 32;

/** 选票密文的字节长度：1 字节版本号 + 两个压缩点。 */
export const ballotCipherSize = 1 + 2 * ballotPointSize;

/** 选票密文的格式版本。 */
export const ballotCipherVersion = 3;

/** 单个提案的选项数上限，同时也是电路里选项下标的位宽上限。 */
export const ballotMaxOptions = 256;

/** 未压缩曲线点的字节长度：X 与 Y 各 32 字节。 */
export const ballotUncompressedPointSize = 2 * ballotPointSize;

/** 曲线上的点（扩展坐标：x = X/Z，y = Y/Z，T = XY/Z）。 */
interface ExtendedPoint {
  x: bigint;
  y: bigint;
  z: bigint;
  t: bigint;
}

/** 仿射曲线点。 */
export interface AffinePoint {
  x: bigint;
  y: bigint;
}

/** 曲线基点（公开值）。 */
export const ballotBasePoint: AffinePoint = { x: baseX, y: baseY };

function mod(value: bigint): bigint {
  const reduced = value % bn254ScalarModulus;
  return reduced < 0n ? reduced + bn254ScalarModulus : reduced;
}

function mulMod(a: bigint, b: bigint): bigint {
  return mod(a * b);
}

function addMod(a: bigint, b: bigint): bigint {
  return mod(a + b);
}

function subMod(a: bigint, b: bigint): bigint {
  return mod(a - b);
}

/** 模逆：费马小定理，a^(r-2) mod r。 */
function invMod(a: bigint): bigint {
  if (a === 0n) {
    throw new Error('cannot invert zero in the scalar field');
  }
  let result = 1n;
  let base = a;
  let exponent = bn254ScalarModulus - 2n;
  while (exponent > 0n) {
    if (exponent & 1n) {
      result = mulMod(result, base);
    }
    base = mulMod(base, base);
    exponent >>= 1n;
  }
  return result;
}

function pointFromAffine(affine: AffinePoint): ExtendedPoint {
  return {
    x: affine.x,
    y: affine.y,
    z: 1n,
    t: mulMod(affine.x, affine.y),
  };
}

function identityPoint(): ExtendedPoint {
  return { x: 0n, y: 1n, z: 1n, t: 0n };
}

/**
 * 点加法（add-2008-hwcd-3，a = -1 的扩展坐标公式）。
 */
function pointAdd(p: ExtendedPoint, q: ExtendedPoint): ExtendedPoint {
  const a = mulMod(subMod(p.y, p.x), subMod(q.y, q.x));
  const b = mulMod(addMod(p.y, p.x), addMod(q.y, q.x));
  const c = mulMod(mulMod(p.t, 2n), mulMod(curveD, q.t));
  const d = mulMod(mulMod(p.z, 2n), q.z);
  const e = subMod(b, a);
  const f = subMod(d, c);
  const g = addMod(d, c);
  const h = addMod(b, a);
  return {
    x: mulMod(e, f),
    y: mulMod(g, h),
    t: mulMod(e, h),
    z: mulMod(f, g),
  };
}

/** 点倍增（dbl-2008-hwcd，a = -1 的扩展坐标公式）。 */
function pointDouble(p: ExtendedPoint): ExtendedPoint {
  const a = mulMod(p.x, p.x);
  const b = mulMod(p.y, p.y);
  const c = mulMod(mulMod(p.z, p.z), 2n);
  const d = mod(curveA * a);
  const e = subMod(subMod(mulMod(addMod(p.x, p.y), addMod(p.x, p.y)), a), b);
  const g = addMod(d, b);
  const f = subMod(g, c);
  const h = subMod(d, b);
  return {
    x: mulMod(e, f),
    y: mulMod(g, h),
    t: mulMod(e, h),
    z: mulMod(f, g),
  };
}

/** 标量乘法：从高位到低位的双倍加法。 */
function pointScalarMul(point: ExtendedPoint, scalar: bigint): ExtendedPoint {
  let result = identityPoint();
  let addend = point;
  let remaining = scalar;
  while (remaining > 0n) {
    if (remaining & 1n) {
      result = pointAdd(result, addend);
    }
    addend = pointDouble(addend);
    remaining >>= 1n;
  }
  return result;
}

function toAffine(point: ExtendedPoint): AffinePoint {
  const zInv = invMod(point.z);
  return {
    x: mulMod(point.x, zInv),
    y: mulMod(point.y, zInv),
  };
}

/** 域元素的 32 字节大端编码。 */
function bigEndianBytes(value: bigint): Uint8Array {
  const bytes = new Uint8Array(ballotPointSize);
  let remaining = value;
  for (let i = ballotPointSize - 1; i >= 0; i -= 1) {
    bytes[i] = Number(remaining & 0xffn);
    remaining >>= 8n;
  }
  return bytes;
}

/**
 * 判断坐标是否「字典序最大」。
 *
 * 与 gnark-crypto 的 `fr.Element.LexicographicallyLargest` 同口径：把元素与
 * 它的相反数（模 r）的大端表示逐字节比较，第一个不同的字节决定大小。点压缩
 * 时用它决定符号位，两侧必须一致，否则解不开。
 */
function isLexicographicallyLargest(value: bigint): boolean {
  const negated = mod(-value);
  const left = bigEndianBytes(value);
  const right = bigEndianBytes(negated);
  for (let i = 0; i < left.length; i += 1) {
    if (left[i] !== right[i]) {
      return left[i] > right[i];
    }
  }
  return false;
}

/**
 * 压缩点编码：Y 的大端表示，最高位放 X 的符号位，整体按小端存放。
 *
 * 与 gnark-crypto 的 twisted Edwards 点编码完全一致（Ed25519 风格）。
 */
export function encodePoint(affine: AffinePoint): Uint8Array {
  const bytes = bigEndianBytes(affine.y);
  if (isLexicographicallyLargest(affine.x)) {
    bytes[0] |= 0x80;
  }
  return bytes.reverse();
}

/** 解析未压缩的曲线点（X 与 Y 各 32 字节大端）。 */
export function decodeUncompressedPoint(bytes: Uint8Array): AffinePoint {
  if (bytes.length !== ballotUncompressedPointSize) {
    throw new Error(
      `uncompressed point must be ${ballotUncompressedPointSize} bytes, got ${bytes.length}`,
    );
  }
  return {
    x: bytesToBigInt(bytes.subarray(0, ballotPointSize)),
    y: bytesToBigInt(bytes.subarray(ballotPointSize)),
  };
}

function bytesToBigInt(bytes: Uint8Array): bigint {
  let value = 0n;
  for (const byte of bytes) {
    value = (value << 8n) | BigInt(byte);
  }
  return value;
}

/** 检查点是否在曲线上：a·x² + y² = 1 + d·x²·y²。 */
export function isOnCurve(affine: AffinePoint): boolean {
  const x2 = mulMod(affine.x, affine.x);
  const y2 = mulMod(affine.y, affine.y);
  const left = addMod(mulMod(curveA, x2), y2);
  const right = addMod(1n, mulMod(mulMod(curveD, x2), y2));
  return left === right;
}

/**
 * 检查点是否落在素数阶子群里。
 *
 * 这条曲线的余因子是 8，小阶点会让解密方泄漏私钥的低位比特。
 */
export function isInSubGroup(affine: AffinePoint): boolean {
  const multiplied = pointScalarMul(pointFromAffine(affine), groupOrder);
  const result = toAffine(multiplied);
  return result.x === 0n && result.y === 1n;
}

/** 采样一个标量域内的随机数（均匀分布，不做按模归约）。 */
export function randomScalar(
  fillRandom: (length: number) => Uint8Array = defaultFillRandom,
): bigint {
  for (;;) {
    const bytes = fillRandom(ballotPointSize);
    const value = bytesToBigInt(bytes);
    if (value >= bn254ScalarModulus) {
      // 超出域外的取值按模归约会破坏均匀分布，直接重采样。
      continue;
    }
    return value;
  }
}

/** 默认的随机源：浏览器加密随机数。 */
function defaultFillRandom(length: number): Uint8Array {
  const bytes = new Uint8Array(length);
  crypto.getRandomValues(bytes);
  return bytes;
}

/**
 * 用提案的计票公钥加密选项下标，返回 v3 选票密文。
 *
 * 密文是 (a, b) = (r·G, m·G + r·Y)：m 是选项下标，r 是一次性随机数，
 * Y 是提案计票公钥。服务端只有密文时无法还原 m。
 *
 * 计票公钥以**未压缩**形式传入（X 与 Y 各 32 字节大端）—— 浏览器端不需要
 * 求平方根来解压点，服务端直接给出坐标更省事，也让客户端能自己做曲线检查。
 */
export function encryptBallot(
  electionPublicKey: Uint8Array,
  optionIndex: number,
  fillRandom: (length: number) => Uint8Array = defaultFillRandom,
): Uint8Array {
  if (optionIndex < 0 || optionIndex >= ballotMaxOptions) {
    throw new Error(`option index out of range: ${optionIndex}`);
  }
  const key = decodeUncompressedPoint(electionPublicKey);
  if (!isOnCurve(key)) {
    throw new Error('election public key is not on the curve');
  }
  if (!isInSubGroup(key)) {
    throw new Error('election public key is not in the prime order subgroup');
  }

  const nonce = randomScalar(fillRandom);
  const base = pointFromAffine({ x: baseX, y: baseY });
  const a = pointScalarMul(base, nonce);
  const b = pointAdd(
    pointScalarMul(base, BigInt(optionIndex)),
    pointScalarMul(pointFromAffine(key), nonce),
  );

  const ciphertext = new Uint8Array(ballotCipherSize);
  ciphertext[0] = ballotCipherVersion;
  ciphertext.set(encodePoint(toAffine(a)), 1);
  ciphertext.set(encodePoint(toAffine(b)), 1 + ballotPointSize);
  return ciphertext;
}

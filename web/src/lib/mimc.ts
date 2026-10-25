/**
 * BN254 标量域上的 MiMC 哈希（与 gnark `std/hash/mimc` 逐位一致）。
 *
 * 电路里 nullifier 的约束是 `nullifier = MiMC(秘密, 提案域元素)`：只有两边
 * 算出同一个值，prover 才能出证、服务端才验得过。所以这里的轮数、轮常量、
 * 轮函数与外层结构都必须与 Go 侧 `ballot/nullifier.go` 完全一致。
 *
 * 轮常量的来源：gnark 用 seed = "seed" 派生一条 Keccak-256 链
 * （`c[i] = keccak(c[i-1])`，起点是 `keccak("seed")`），共 91 个。这里照搬
 * 同一套派生，而不是把常量抄成一张表 —— 抄表迟早会抄错，而抄错不会报错。
 */

import { keccak256 } from './keccak';
import { addField, bytesToBigInt, modField, mulField } from './field';

/** MiMC 的轮数。 */
export const mimcRoundCount = 91;

/** 派生轮常量的种子，与 gnark 的 MiMC 一致。 */
const MIMC_SEED = 'seed';

let cachedConstants: bigint[] | null = null;

/**
 * MiMC 的 91 个轮常量（域元素）。
 *
 * 派生方式与 gnark 的 `mimc.GetConstants()` 一致：`keccak("seed")` 之后
 * 反复哈希上一轮的结果，每轮取一次摘要并按模 r 归约。
 */
export function mimcConstants(): bigint[] {
  if (cachedConstants) {
    return cachedConstants;
  }
  const constants: bigint[] = [];
  let current = keccak256(new TextEncoder().encode(MIMC_SEED));
  for (let i = 0; i < mimcRoundCount; i += 1) {
    current = keccak256(current);
    constants.push(modField(bytesToBigInt(current)));
  }
  cachedConstants = constants;
  return constants;
}

/**
 * MiMC 哈希：轮函数 `x = (x + h + c)^5`，外层是 Miyaguchi–Preneel
 * （`h = h + E(x) + x`）。
 *
 * @param elements 依次参与哈希的域元素
 * @returns 哈希结果（域元素）
 */
export function mimcHash(...elements: bigint[]): bigint {
  const constants = mimcConstants();
  let h = 0n;
  for (const element of elements) {
    let m = element;
    for (let i = 0; i < constants.length; i += 1) {
      const sum = addField(addField(m, h), constants[i]);
      const square = mulField(sum, sum);
      const fourth = mulField(square, square);
      m = mulField(fourth, sum);
    }
    m = addField(m, h);
    h = addField(addField(h, m), element);
  }
  return h;
}

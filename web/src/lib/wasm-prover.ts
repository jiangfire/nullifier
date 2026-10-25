/**
 * 浏览器端出票的主线程封装。
 *
 * 真正干活的是 Worker 里的 WASM 模块（`cmd/ballot-wasm`）：加密、nullifier
 * 派生、公开输入拼装、Groth16 证明全在 Go 侧完成，门户只负责取资源、传选项
 * 下标、拿结果。这样客户端与服务端共用同一套代码，不会出现「两侧公开输入
 * 拼得不一样导致证明验不过」。
 *
 * 依赖的三个静态资源由 `make web-wasm` 与 `make web-proving-key` 放到
 * 门户的静态目录：
 *
 * - `wasm_exec.js`   Go 官方胶水
 * - `ballot-prover.wasm`  WASM 模块
 * - `proving.key`    选票电路的 proving key（只在客户端侧）
 */

import type {
  ProverResult,
  ProverWorkerMessage,
  ProverWorkerReply,
} from './wasm-prover-protocol';

/** 出票所需的参数。 */
export interface LocalBallotRequest {
  /** 提案 ID 原文。 */
  proposalId: string;
  /** 计票公钥的 base64 编码（未压缩）。 */
  electionPublicKey: string;
  /** 选中的选项下标。 */
  optionIndex: number;
  /** 提案的选项数。 */
  numOptions: number;
}

/** 出票结果。 */
export interface LocalBallotResult {
  /** 选票密文的 base64 编码。 */
  ciphertext: string;
  /** 32 字节 nullifier 的小写 hex。 */
  nullifier: string;
  /** Groth16 证明的 base64 编码。 */
  proof: string;
  /** 加载 proving key 并编译电路的耗时（毫秒）。 */
  initMs: number;
  /** 出证明的耗时（毫秒）。 */
  proveMs: number;
}

/** 静态资源的地址：相对当前页面解析，门户部署在子路径下也能取到。 */
export function assetUrl(name: string, base: string = document.baseURI): string {
  return new URL(name, base).href;
}

/** 把 WASM 模块的结果整理成门户要用的形状。 */
export function toLocalBallotResult(result: ProverResult): LocalBallotResult {
  if (!result.ok) {
    throw new Error(result.error ?? '浏览器端生成选票失败');
  }
  if (!result.ciphertext || !result.nullifier || !result.proof) {
    throw new Error('浏览器端生成选票失败：结果不完整');
  }
  return {
    ciphertext: result.ciphertext,
    nullifier: result.nullifier,
    proof: result.proof,
    initMs: result.init_ms ?? 0,
    proveMs: result.prove_ms ?? 0,
  };
}

/**
 * 在浏览器本地生成一张选票。
 *
 * 全程在本地完成：密文、nullifier 与证明都不经过服务端，服务端拿到的只有
 * 密文、nullifier 与证明本身，看不到选了哪个选项。
 */
export async function generateLocalBallot(
  request: LocalBallotRequest,
): Promise<LocalBallotResult> {
  const worker = new Worker(new URL('../workers/ballot-prover.worker.ts', import.meta.url));
  try {
    const message: ProverWorkerMessage = {
      type: 'prove',
      glueUrl: assetUrl('wasm_exec.js'),
      wasmUrl: assetUrl('ballot-prover.wasm'),
      provingKeyUrl: assetUrl('proving.key'),
      request: {
        proposal_id: request.proposalId,
        election_public_key: request.electionPublicKey,
        option_index: request.optionIndex,
        num_options: request.numOptions,
      },
    };
    const result = await new Promise<ProverResult>((resolve, reject) => {
      worker.onmessage = (event: MessageEvent<ProverWorkerReply>) => {
        resolve(event.data.result);
      };
      worker.onerror = (event: ErrorEvent) => {
        reject(new Error(event.message || '浏览器端 prover 启动失败'));
      };
      worker.postMessage(message);
    });
    return toLocalBallotResult(result);
  } finally {
    worker.terminate();
  }
}

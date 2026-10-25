/**
 * 浏览器端出**资格证明**的主线程封装。
 *
 * 资格证明回答的是「我在资格名单里」，而不是「我选了哪个选项」。它由同一
 * 个 WASM 模块（`cmd/ballot-wasm`）里的第二套电路产出，密钥也是另一把
 * （`eligibility-proving.key`）。
 *
 * 门户这一侧**不做任何密码学**：成员包是成员自己拿到的 JSON 文件，导入后
 * 原样交给 WASM，拿回「证明 + 去重键」两样东西。
 *
 * 依赖的静态资源：
 *
 * - `wasm_exec.js`            Go 官方胶水
 * - `ballot-prover.wasm`      WASM 模块（选票与资格两套电路都在里面）
 * - `eligibility-proving.key` 资格电路的 proving key（只在客户端侧）
 */

import type {
  EligibilityMemberPackage,
  EligibilityProverResult,
  EligibilityProveRequest,
  ProverWorkerMessage,
  ProverWorkerReply,
} from './wasm-prover-protocol';
import { assetUrl } from './wasm-prover';

/** 出资格证明所需的参数。 */
export interface EligibilityProofRequest {
  /** 提案 ID 原文。 */
  proposalId: string;
  /** 服务端签发的领取挑战（十六进制的域元素）。 */
  challenge: string;
  /** 服务端配置的名单树根（十六进制的域元素）。 */
  root: string;
  /** 成员自己的资格包。 */
  memberPackage: EligibilityMemberPackage;
}

/** 资格证明的结果。 */
export interface EligibilityProofResult {
  /** 资格证明的 base64 编码。 */
  proof: string;
  /** 去重键的十六进制编码。 */
  qualificationId: string;
  /** 加载 proving key 并编译电路的耗时（毫秒）。 */
  initMs: number;
  /** 出证明的耗时（毫秒）。 */
  proveMs: number;
}

/**
 * 把门户的参数拼成 WASM 模块的入参。
 *
 * 字段名与 Go 侧 `cmd/ballot-wasm` 的 `eligibilityProveRequest` 一一对应 ——
 * 跨语言锁死：`tests/architecture/eligibility_wire_test.go` 会拿两边的字段名
 * 对照，改了一边不改另一边就会红。
 */
export function buildEligibilityProveRequest(
  request: EligibilityProofRequest,
): EligibilityProveRequest {
  return {
    proposal_id: request.proposalId,
    challenge: request.challenge,
    root: request.root,
    member_package: request.memberPackage,
  };
}

/**
 * 在浏览器本地生成一份资格证明。
 *
 * 全程在本地完成：资格秘密、Merkle 路径与 proving key 都不出浏览器。服务端
 * 拿到的只有「证明 + 去重键 + 挑战」—— 它据此强制一人一票，却不知道这份
 * 资格属于谁。
 */
export async function generateEligibilityProof(
  request: EligibilityProofRequest,
): Promise<EligibilityProofResult> {
  const worker = new Worker(new URL('../workers/ballot-prover.worker.ts', import.meta.url));
  try {
    const message: ProverWorkerMessage = {
      type: 'proveEligibility',
      glueUrl: assetUrl('wasm_exec.js'),
      wasmUrl: assetUrl('ballot-prover.wasm'),
      provingKeyUrl: assetUrl('eligibility-proving.key'),
      request: buildEligibilityProveRequest(request),
    };
    const result = await new Promise<EligibilityProverResult>((resolve, reject) => {
      worker.onmessage = (event: MessageEvent<ProverWorkerReply<EligibilityProverResult>>) => {
        resolve(event.data.result);
      };
      worker.onerror = (event: ErrorEvent) => {
        reject(new Error(event.message || '浏览器端资格 prover 启动失败'));
      };
      worker.postMessage(message);
    });
    if (!result.ok) {
      throw new Error(result.error ?? '浏览器端生成资格证明失败');
    }
    if (!result.proof || !result.qualification_id) {
      throw new Error('浏览器端生成资格证明失败：结果不完整');
    }
    return {
      proof: result.proof,
      qualificationId: result.qualification_id,
      initMs: result.init_ms ?? 0,
      proveMs: result.prove_ms ?? 0,
    };
  } finally {
    worker.terminate();
  }
}

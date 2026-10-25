/**
 * 浏览器端 prover 的消息协议。
 *
 * 主线程与 Web Worker 之间的契约：主线程把「资源地址 + 选票参数」发过去，
 * Worker 回一份结果。类型单独放一个文件，是为了让 Worker 文件只被当作
 * Worker 入口打包，不会因为共享类型而混进主线程 chunk。
 */

/** 出票请求（字段名与 WASM 模块的 JSON 入参一致）。 */
export interface ProverRequest {
  /** 提案 ID 原文。 */
  proposal_id: string;
  /** 计票公钥的 base64 编码（未压缩，X 与 Y 各 32 字节大端）。 */
  election_public_key: string;
  /** 选中的选项下标。 */
  option_index: number;
  /** 提案的选项数。 */
  num_options: number;
}

/** WASM 模块的出票结果。 */
export interface ProverResult {
  ok: boolean;
  error?: string;
  /** 选票密文的 base64 编码。 */
  ciphertext?: string;
  /** 32 字节 nullifier 的小写 hex。 */
  nullifier?: string;
  /** Groth16 证明的 base64 编码。 */
  proof?: string;
  /** 加载 proving key 并编译电路的耗时（毫秒）。 */
  init_ms?: number;
  /** 出证明的耗时（毫秒）。 */
  prove_ms?: number;
}

/** 主线程发给 Worker 的消息。 */
export interface ProverWorkerMessage {
  type: 'prove';
  /** Go 官方胶水 wasm_exec.js 的地址。 */
  glueUrl: string;
  /** WASM 模块的地址。 */
  wasmUrl: string;
  /** proving key 的地址。 */
  provingKeyUrl: string;
  request: ProverRequest;
}

/** Worker 回给主线程的消息。 */
export interface ProverWorkerReply {
  type: 'result';
  result: ProverResult;
}

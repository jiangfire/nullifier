/**
 * 浏览器端 prover 的消息协议。
 *
 * 主线程与 Web Worker 之间的契约：主线程把「资源地址 + 参数」发过去，
 * Worker 回一份结果。类型单独放一个文件，是为了让 Worker 文件只被当作
 * Worker 入口打包，不会因为共享类型而混进主线程 chunk。
 *
 * 模块里有**两套**电路，密钥互相独立：
 *
 * - 选票：选票电路的 proving key（`proving.key`）
 * - 资格：资格电路的 proving key（`eligibility-proving.key`）
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

/**
 * 成员资格包。
 *
 * 字段名与 Go 侧 `eligibility.MemberPackage` 一一对应：门户只做 JSON 解析与
 * 转发，密码学全在 WASM 里。
 */
export interface EligibilityMemberPackage {
  /** 成员标识（工号、选民编号……）。服务端看不到它。 */
  member_id: string;
  /** 资格秘密（32 字节随机值的十六进制，规范编码）。 */
  secret: string;
  /** 叶子 = MiMC(秘密)（十六进制的域元素）。 */
  leaf: string;
  /** 叶子在名单树里的位置。 */
  index: number;
  /** Merkle 路径（兄弟节点，十六进制）。 */
  path: string[];
  /** 名单树根（十六进制）。 */
  root: string;
  /** 树深。 */
  depth: number;
}

/** 资格证明请求（字段名与 WASM 模块的 JSON 入参一致）。 */
export interface EligibilityProveRequest {
  /** 提案 ID 原文。 */
  proposal_id: string;
  /** 服务端签发的领取挑战（十六进制的域元素）。 */
  challenge: string;
  /** 服务端配置的名单树根（十六进制的域元素）。 */
  root: string;
  /** 成员自己的资格包。 */
  member_package: EligibilityMemberPackage;
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

/** WASM 模块的资格证明结果。 */
export interface EligibilityProverResult {
  ok: boolean;
  error?: string;
  /** 资格证明的 base64 编码。 */
  proof?: string;
  /** 去重键的十六进制编码。 */
  qualification_id?: string;
  /** 加载 proving key 并编译电路的耗时（毫秒）。 */
  init_ms?: number;
  /** 出证明的耗时（毫秒）。 */
  prove_ms?: number;
}

/** 主线程发给 Worker 的消息。 */
export type ProverWorkerMessage =
  | {
      type: 'prove';
      /** Go 官方胶水 wasm_exec.js 的地址。 */
      glueUrl: string;
      /** WASM 模块的地址。 */
      wasmUrl: string;
      /** 选票 proving key 的地址。 */
      provingKeyUrl: string;
      request: ProverRequest;
    }
  | {
      type: 'proveEligibility';
      /** Go 官方胶水 wasm_exec.js 的地址。 */
      glueUrl: string;
      /** WASM 模块的地址。 */
      wasmUrl: string;
      /** 资格 proving key 的地址。 */
      provingKeyUrl: string;
      request: EligibilityProveRequest;
    };

/** Worker 回给主线程的消息。 */
export interface ProverWorkerReply<
  T extends ProverResult | EligibilityProverResult = ProverResult,
> {
  type: 'result';
  result: T;
}

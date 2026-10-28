/**
 * 浏览器端 prover 的 Worker。
 *
 * 为什么放 Worker 里：加载 proving key 会编译一次电路，浏览器上大约
 * 7~8 秒；出证明再花 2 秒左右。这两段都在主线程上做会把界面卡死，
 * 放进 Worker 里主线程照常响应。
 *
 * 这里的密码学全部由 WASM 模块（`cmd/ballot-wasm`）完成：加密、nullifier
 * 派生、公开输入拼装、Groth16 证明、资格证明。本文件只负责把资源喂进去、
 * 把结果取回来。
 *
 * 同一个模块里有两套电路（选票与资格），密钥互相独立：走哪条路就下载哪把
 * proving key。
 */

import type {
  EligibilityProverResult,
  ProverResult,
  ProverWorkerMessage,
  ProverWorkerReply,
} from '../lib/wasm-prover-protocol';

/** Go 官方胶水在全局对象上挂的运行时。 */
interface GoRuntime {
  importObject: WebAssembly.Imports;
  run(instance: WebAssembly.Instance): Promise<void>;
}

declare const Go: new () => GoRuntime;
declare function importScripts(urls: string): void;

/** WASM 模块在全局对象上暴露的 API。 */
interface ProverApi {
  init(payload: string): { ok: boolean; error?: string; init_ms?: number };
  prove(payload: string): ProverResult;
  initEligibility(payload: string): { ok: boolean; error?: string; init_ms?: number };
  proveEligibility(payload: string): EligibilityProverResult;
}

const scope = self as unknown as {
  nullifierProver?: ProverApi;
  postMessage(message: ProverWorkerReply): void;
  onmessage: ((event: MessageEvent<ProverWorkerMessage>) => void) | null;
};

/** 字节数组转 base64：分块处理，避免大数组展开爆栈。 */
function toBase64(bytes: Uint8Array): string {
  const chunkSize = 0x8000;
  let binary = '';
  for (let offset = 0; offset < bytes.length; offset += chunkSize) {
    binary += String.fromCharCode(...bytes.subarray(offset, offset + chunkSize));
  }
  return btoa(binary);
}

/** 等 WASM 模块把 API 注册到全局对象上。 */
async function waitForApi(): Promise<ProverApi> {
  for (let attempt = 0; attempt < 200; attempt += 1) {
    if (scope.nullifierProver) {
      return scope.nullifierProver;
    }
    await new Promise((resolve) => setTimeout(resolve, 20));
  }
  throw new Error('WASM 模块没有注册 prover API');
}

/** 跑一次完整的出票流程。 */
async function runProver(message: ProverWorkerMessage): Promise<ProverResult | EligibilityProverResult> {
  importScripts(message.glueUrl);

  const go = new Go();
  const wasmResponse = await fetch(message.wasmUrl);
  if (!wasmResponse.ok) {
    throw new Error(`下载 WASM 模块失败：HTTP ${wasmResponse.status}`);
  }
  const wasmBytes = await wasmResponse.arrayBuffer();
  const { instance } = await WebAssembly.instantiate(wasmBytes, go.importObject);
  // go.run 不会返回：Go 运行时必须一直在，JS 才能随时回调进去。
  void go.run(instance);

  const api = await waitForApi();

  const keyResponse = await fetch(message.provingKeyUrl);
  if (!keyResponse.ok) {
    throw new Error(`下载 proving key 失败：HTTP ${keyResponse.status}`);
  }
  const provingKey = toBase64(new Uint8Array(await keyResponse.arrayBuffer()));

  if (message.type === 'proveEligibility') {
    // proving key 是按名单树深生成的：深度以**成员包**为准（名单工具生成时
    // 写入），缺省（老成员包没有深度字段以外的情况）由 WASM 侧取默认值 10。
    // 深度不匹配会在出证明这一步当场失败 —— 宁可失败也不出注定验不过的证明。
    const initResult = api.initEligibility(
      JSON.stringify({
        proving_key: provingKey,
        depth: message.request.member_package.depth,
      }),
    );
    if (!initResult.ok) {
      throw new Error(initResult.error ?? '加载资格 proving key 失败');
    }
    const result = api.proveEligibility(JSON.stringify(message.request));
    return { ...result, init_ms: initResult.init_ms };
  }

  const initResult = api.init(JSON.stringify({ proving_key: provingKey }));
  if (!initResult.ok) {
    throw new Error(initResult.error ?? '加载 proving key 失败');
  }

  const result = api.prove(JSON.stringify(message.request));
  return { ...result, init_ms: initResult.init_ms };
}

scope.onmessage = (event: MessageEvent<ProverWorkerMessage>) => {
  const message = event.data;
  if (!message || (message.type !== 'prove' && message.type !== 'proveEligibility')) {
    return;
  }
  runProver(message)
    .then((result) => {
      scope.postMessage({ type: 'result', result });
    })
    .catch((error: unknown) => {
      const detail = error instanceof Error ? error.message : String(error);
      scope.postMessage({ type: 'result', result: { ok: false, error: detail } });
    });
};

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
 * 同一个模块里有两套电路（选票与资格），密钥互相独立。Worker 由主线程
 * **常驻复用**（见 `lib/prover-client.ts`），所以这里把贵的东西全部缓存：
 *
 * - Go 运行时 + WASM 实例：同一对地址只加载一次（下载 11.9 MB + 实例化）；
 * - proving key：同一地址只下载一次（约 1.2 MB）；
 * - 电路初始化（`init` / `initEligibility`，编译电路的大头）：同一把 key
 *   （资格另加树深）只做一次 —— Go 侧选票与资格是两个独立变量，互不覆盖，
 *   「领票 → 投票」交替调用也不需要重新 init。
 */

import { fetchWithTimeout } from '../lib/fetch-with-timeout';
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

/**
 * 静态资源下载的超时：wasm 约 2.8 MB（gzip）、proving key 约 1.2 MB，
 * 内网是秒级、公网慢链路也要给足；但网络**停滞**时必须在 60 秒内失败，
 * 否则加载态永远收不了场。
 */
const FETCH_TIMEOUT_MS = 60_000;

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

/** 已装载的 WASM 运行时（同一对 glue/wasm 地址只装一份）。 */
let runtime: { glueUrl: string; wasmUrl: string; api: ProverApi } | null = null;

async function loadRuntime(glueUrl: string, wasmUrl: string): Promise<ProverApi> {
  if (runtime !== null && runtime.glueUrl === glueUrl && runtime.wasmUrl === wasmUrl) {
    return runtime.api;
  }
  importScripts(glueUrl);

  const go = new Go();
  const wasmResponse = await fetchWithTimeout(wasmUrl, FETCH_TIMEOUT_MS);
  if (!wasmResponse.ok) {
    throw new Error(`下载 WASM 模块失败：HTTP ${wasmResponse.status}`);
  }
  const wasmBytes = await wasmResponse.arrayBuffer();
  const { instance } = await WebAssembly.instantiate(wasmBytes, go.importObject);
  // go.run 不会返回：Go 运行时必须一直在，JS 才能随时回调进去。
  void go.run(instance);

  const api = await waitForApi();
  runtime = { glueUrl, wasmUrl, api };
  return api;
}

/** proving key 的 base64 缓存：同一地址只下载一次。 */
const provingKeyCache = new Map<string, string>();

async function loadProvingKey(provingKeyUrl: string): Promise<string> {
  const cached = provingKeyCache.get(provingKeyUrl);
  if (cached !== undefined) {
    return cached;
  }
  const keyResponse = await fetchWithTimeout(provingKeyUrl, FETCH_TIMEOUT_MS);
  if (!keyResponse.ok) {
    throw new Error(`下载 proving key 失败：HTTP ${keyResponse.status}`);
  }
  const encoded = toBase64(new Uint8Array(await keyResponse.arrayBuffer()));
  provingKeyCache.set(provingKeyUrl, encoded);
  return encoded;
}

/** 选票电路当前装载的 proving key 地址；资格电路另记树深（密钥按深度生成）。 */
let ballotKeyUrl: string | null = null;
let eligibilityKeyUrl: string | null = null;
let eligibilityDepth: number | null = null;

/** 跑一次完整的出票流程；缓存命中时 init 一秒都不用等。 */
async function runProver(message: ProverWorkerMessage): Promise<ProverResult | EligibilityProverResult> {
  const api = await loadRuntime(message.glueUrl, message.wasmUrl);

  if (message.type === 'proveEligibility') {
    // proving key 是按名单树深生成的：深度以**成员包**为准（名单工具生成时
    // 写入）。深度不匹配会在出证明这一步当场失败 —— 宁可失败也不出注定
    // 验不过的证明。
    const depth = message.request.member_package.depth;
    let initMs = 0;
    if (eligibilityKeyUrl !== message.provingKeyUrl || eligibilityDepth !== depth) {
      const provingKey = await loadProvingKey(message.provingKeyUrl);
      const initResult = api.initEligibility(
        JSON.stringify({
          proving_key: provingKey,
          depth,
        }),
      );
      if (!initResult.ok) {
        throw new Error(initResult.error ?? '加载资格 proving key 失败');
      }
      eligibilityKeyUrl = message.provingKeyUrl;
      eligibilityDepth = depth;
      initMs = initResult.init_ms ?? 0;
    }
    const result = api.proveEligibility(JSON.stringify(message.request));
    return { ...result, init_ms: initMs };
  }

  let initMs = 0;
  if (ballotKeyUrl !== message.provingKeyUrl) {
    const provingKey = await loadProvingKey(message.provingKeyUrl);
    const initResult = api.init(JSON.stringify({ proving_key: provingKey }));
    if (!initResult.ok) {
      throw new Error(initResult.error ?? '加载 proving key 失败');
    }
    ballotKeyUrl = message.provingKeyUrl;
    initMs = initResult.init_ms ?? 0;
  }

  const result = api.prove(JSON.stringify(message.request));
  return { ...result, init_ms: initMs };
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

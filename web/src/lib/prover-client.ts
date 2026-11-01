/**
 * 常驻 prover Worker 的主线程入口。
 *
 * 出一次证明要下载 11.9 MB 的 wasm、编译一次电路（合计约 7~10 秒）。此前
 * 每次证明都新开 Worker 从零来一遍，领一次票（资格证明）+ 投一次票（选票
 * 证明）就要付两次全价。现在**整个页面共用一个 Worker**：Worker 侧把
 * Go 运行时、proving key 与电路初始化缓存住（见 `ballot-prover.worker.ts`），
 * 这里把请求**串行**排队 —— Worker 的回复不带请求标识，串行才能把回复
 * 对回请求；两条证明路径（选票 / 资格）共用这一个队列。
 *
 * Worker 崩溃时丢弃实例让在途请求失败，下一个请求自动重新拉起一个
 * （缓存随之丢失，慢一次但正确）。
 */
import type {
  ProverResult,
  ProverWorkerMessage,
  ProverWorkerReply,
} from './wasm-prover-protocol';

let worker: Worker | null = null;
let inflight: ((reply: ProverWorkerReply) => void) | null = null;
let chain: Promise<unknown> = Promise.resolve();

function ensureWorker(): Worker {
  if (worker !== null) {
    return worker;
  }
  const created = new Worker(new URL('../workers/ballot-prover.worker.ts', import.meta.url));
  created.onmessage = (event: MessageEvent<ProverWorkerReply>) => {
    const deliver = inflight;
    inflight = null;
    deliver?.(event.data);
  };
  created.onerror = (event: ErrorEvent) => {
    const deliver = inflight;
    inflight = null;
    worker = null;
    deliver?.({
      type: 'result',
      result: { ok: false, error: event.message || '浏览器端 prover 启动失败' },
    });
  };
  worker = created;
  return created;
}

/** 发一条出证明请求；与其它请求串行执行。 */
export function callProverWorker<T extends ProverResult>(
  message: ProverWorkerMessage,
): Promise<T> {
  const task = chain.then(
    () =>
      new Promise<T>((resolve) => {
        const current = ensureWorker();
        inflight = (reply) => resolve(reply.result as T);
        current.postMessage(message);
      }),
  );
  // 队列吞掉失败：一个请求失败不能让排在后面的请求永远等不到。
  chain = task.then(
    () => undefined,
    () => undefined,
  );
  return task;
}

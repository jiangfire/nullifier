/**
 * 带超时的 fetch。
 *
 * Worker 里下载 wasm / proving key 没有超时的话，网络停滞会让 promise
 * 永不 settle：加载态永远停在 true、按钮永久禁用、主线程的清理逻辑
 * （terminate）也永远不会跑。给每次下载一个上限，停滞时必然失败，
 * 上层才能给出错误并收场。
 *
 * 用 AbortController 而不是 `AbortSignal.timeout`：后者要较新的浏览器，
 * 门户还要照顾内网环境里的老 Chromium。
 */
export async function fetchWithTimeout(url: string, timeoutMs: number): Promise<Response> {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);
  try {
    return await fetch(url, { signal: controller.signal });
  } finally {
    clearTimeout(timer);
  }
}

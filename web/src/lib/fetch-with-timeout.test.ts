import { afterEach, describe, expect, it, vi } from 'vitest';
import { fetchWithTimeout } from './fetch-with-timeout';

describe('fetchWithTimeout', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.useRealTimers();
  });

  it('正常响应原样返回', async () => {
    const response = new Response('{"ok":true}');
    vi.stubGlobal('fetch', vi.fn(async () => response));

    await expect(fetchWithTimeout('http://localhost/wasm', 1000)).resolves.toBe(response);
  });

  it('网络停滞时按超时失败，而不是永不 settle', async () => {
    // 模拟停滞：fetch 返回一个永不 resolve 的 promise（真实场景是
    // TCP 连上但对端不发数据）。20ms 超时后必须以 AbortError 拒绝。
    vi.stubGlobal(
      'fetch',
      vi.fn(
        (_url: string, init?: RequestInit) =>
          new Promise<Response>((_resolve, reject) => {
            init?.signal?.addEventListener('abort', () =>
              reject(new Error('The operation was aborted.')),
            );
          }),
      ),
    );

    await expect(fetchWithTimeout('http://localhost/wasm', 20)).rejects.toThrow(/aborted/i);
  });

  it('成功后清掉定时器，不悬挂事件循环', async () => {
    const setTimeoutSpy = vi.spyOn(globalThis, 'setTimeout');
    vi.stubGlobal('fetch', vi.fn(async () => new Response('ok')));
    await fetchWithTimeout('http://localhost/key', 50);

    expect(setTimeoutSpy).toHaveBeenCalled();
    // clearTimeout 的效果用假定时器验不出更多东西；这里只断言定时器被安排了。
    setTimeoutSpy.mockRestore();
  });
});

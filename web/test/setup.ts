/**
 * 测试环境里的浏览器全局对象。
 *
 * node 环境没有 DOM，但 `useVotePortal()` 在 setup 期间就会读
 * `localStorage` / `sessionStorage`，通知与路由同步还会用到 `window`。
 * 这里补一套最小实现 —— 只够跑起来，不模拟浏览器的任何真实行为。
 */

const storage = new Map<string, string>();
Object.defineProperty(globalThis, 'localStorage', {
  value: {
    getItem: (key: string) => storage.get(key) ?? null,
    setItem: (key: string, value: string) => {
      storage.set(key, value);
    },
    removeItem: (key: string) => {
      storage.delete(key);
    },
    clear: () => {
      storage.clear();
    },
  },
  configurable: true,
});

const sessionStore = new Map<string, string>();
Object.defineProperty(globalThis, 'sessionStorage', {
  value: {
    getItem: (key: string) => sessionStore.get(key) ?? null,
    setItem: (key: string, value: string) => {
      sessionStore.set(key, value);
    },
    removeItem: (key: string) => {
      sessionStore.delete(key);
    },
    clear: () => {
      sessionStore.clear();
    },
  },
  configurable: true,
});

Object.defineProperty(globalThis, 'window', {
  value: {
    setTimeout: (handler: () => void, timeout?: number) =>
      setTimeout(handler, timeout) as unknown as number,
    clearTimeout: (timer: number) => clearTimeout(timer),
    location: {
      href: 'http://localhost:3000/',
      search: '',
      origin: 'http://localhost:3000',
    },
  },
  configurable: true,
});

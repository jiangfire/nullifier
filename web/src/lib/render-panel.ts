/**
 * 面板渲染测试的助手。
 *
 * 门户的 7 个 `.vue` 面板此前**既没有类型检查也没有任何测试**（`tsc` 不认
 * `.vue` 文件），模板里的笔误只能在运行时暴露。这里用 Vue 的服务端渲染
 * 把面板渲染成 HTML 字符串 —— 模板、setup、computed 都会真的跑一遍。
 *
 * **这不是浏览器渲染**：没有 CSS、没有事件绑定、没有客户端挂载。它能抓住的是
 * 「模板引用了不存在的变量」「setup 抛错」「条件渲染分支写反」这一类问题。
 */

import { createSSRApp, reactive, type Component } from 'vue';
import { renderToString } from 'vue/server-renderer';
import { useVotePortal } from '../composables/useVotePortal';
import type { PortalApi } from '../types/portal-api';

/**
 * 造一个面板用的 portal 状态。
 *
 * 与 `App.vue` 里的做法一致：`reactive(useVotePortal())` —— 面板拿到的是
 * 解包后的最终值，不是 Ref。
 *
 * 管理令牌**显式清空**：composable 在 setup 时从 `sessionStorage` 读它，而
 * `sessionStorage` 是跨用例共享的（上一个用例写进去的令牌会漏到下一个）。
 * 需要令牌的用例自己显式设置。
 */
export function createPortalApi(): PortalApi {
  const portal = reactive(useVotePortal()) as PortalApi;
  portal.adminBearerToken = '';
  return portal;
}

/** 把组件渲染成 HTML 字符串。 */
export async function renderComponent(
  component: Component,
  props: Record<string, unknown>,
): Promise<string> {
  return renderToString(createSSRApp(component, props));
}

/** 渲染一个面板（portal 是唯一的 prop）。 */
export function renderPanel(
  component: Component,
  portal: PortalApi,
): Promise<string> {
  return renderComponent(component, { portal });
}

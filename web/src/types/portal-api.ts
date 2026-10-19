import type { UnwrapNestedRefs } from 'vue';
import type { useVotePortal } from '../composables/useVotePortal';

/**
 * 组件层拿到的门户状态视图。
 *
 * `useVotePortal()` 返回的是带 ref 的对象，App.vue 用 `reactive()` 包裹后
 * 再传给各面板；这里复刻同样的解包结果，保证面板里 `portal.healthState.ok`
 * 这类访问在类型层面就是最终值，而不是 Ref。
 */
export type PortalApi = UnwrapNestedRefs<ReturnType<typeof useVotePortal>>;

import type { OpsStepId, SectionId, WorkspaceId } from '../types/portal';

/** 一级页面的标题，导航与顶栏共用同一份口径。 */
export const PAGE_TITLES: Record<SectionId, string> = {
  overview: '概览',
  setup: '连接配置',
  guide: '术语参考',
  hall: '投票大厅',
  identity: '我的身份',
  vote: '参与投票',
  ops: '计票与审计',
};

/** 各工作区的入口页面。 */
export const WORKSPACE_ENTRY: Record<WorkspaceId, SectionId> = {
  user: 'hall',
  admin: 'ops',
};

const WORKSPACE_SECTIONS: Record<WorkspaceId, SectionId[]> = {
  user: ['hall', 'identity', 'vote'],
  admin: ['ops'],
};

const SHARED_SECTIONS: SectionId[] = ['overview', 'setup', 'guide'];

/** 运营端「计票与审计」的六个步骤顺序。 */
export const OPS_STEPS: Array<{ id: OpsStepId; title: string }> = [
  { id: 'create', title: '创建提案' },
  { id: 'decrypt', title: '初始化解密' },
  { id: 'publish', title: '发布结果' },
  { id: 'result', title: '查询结果' },
  { id: 'credential', title: '凭证状态' },
  { id: 'anchor', title: '审计锚点' },
];

/** 某个分区归属哪个工作区；共享分区返回 null。 */
export function resolveWorkspaceForSection(
  section: SectionId,
): WorkspaceId | null {
  if (SHARED_SECTIONS.includes(section)) {
    return null;
  }
  for (const [workspace, sections] of Object.entries(
    WORKSPACE_SECTIONS,
  ) as Array<[WorkspaceId, SectionId[]]>) {
    if (sections.includes(section)) {
      return workspace;
    }
  }
  return null;
}

/** 切换工作区后应该落到哪个页面。 */
export function resolveWorkspaceEntry(workspace: WorkspaceId): SectionId {
  return WORKSPACE_ENTRY[workspace];
}

/**
 * 引导步骤的下一步下标。
 * 返回 null 表示已经到最后一步（应当结束引导）。
 */
export function resolveNextGuideIndex(
  currentIndex: number,
  total: number,
): number | null {
  if (total <= 0) {
    return null;
  }
  if (currentIndex >= total - 1) {
    return null;
  }
  return currentIndex + 1;
}

/** 顶部状态条里的提案短标签。 */
export function formatProposalTag(proposalId: string): string {
  const trimmed = proposalId.trim();
  if (trimmed === '') {
    return '未选提案';
  }
  return `#${trimmed.slice(0, 6)}`;
}

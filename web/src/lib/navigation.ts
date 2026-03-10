import type { PortalRouteState, SectionId, WorkspaceId } from '../types/portal';

export const DEFAULT_WORKSPACE: WorkspaceId = 'user';

const workspaceSectionMap: Record<WorkspaceId, SectionId[]> = {
  user: ['hall', 'identity', 'vote'],
  admin: ['ops'],
};

export function getDefaultSection(workspace: WorkspaceId): SectionId {
  return workspaceSectionMap[workspace][0];
}

export function isWorkspace(value: string): value is WorkspaceId {
  return value === 'user' || value === 'admin';
}

export function isSectionAllowed(workspace: WorkspaceId, section: SectionId): boolean {
  return workspaceSectionMap[workspace].includes(section);
}

function normalizeWorkspace(value: string | null | undefined): WorkspaceId {
  if (value && isWorkspace(value)) {
    return value;
  }
  return DEFAULT_WORKSPACE;
}

function normalizeSection(value: string | null | undefined, workspace: WorkspaceId): SectionId {
  if (value === 'hall' || value === 'identity' || value === 'vote' || value === 'ops') {
    if (isSectionAllowed(workspace, value)) {
      return value;
    }
  }
  return getDefaultSection(workspace);
}

function normalizeProposalId(value: string | null | undefined): string {
  if (!value) {
    return '';
  }
  return value.trim();
}

export function parsePortalRoute(search: string): PortalRouteState {
  const params = new URLSearchParams(search);
  const workspace = normalizeWorkspace(params.get('ws'));
  const section = normalizeSection(params.get('tab'), workspace);
  const proposalId = normalizeProposalId(params.get('proposal'));
  return { workspace, section, proposalId };
}

export function buildPortalSearch(state: PortalRouteState): string {
  const workspace = normalizeWorkspace(state.workspace);
  const section = normalizeSection(state.section, workspace);
  const proposalId = normalizeProposalId(state.proposalId);

  const params = new URLSearchParams();
  params.set('ws', workspace);
  params.set('tab', section);
  if (proposalId !== '') {
    params.set('proposal', proposalId);
  }
  return params.toString();
}

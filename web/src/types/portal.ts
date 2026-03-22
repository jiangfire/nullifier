export type WorkspaceId = 'user' | 'admin';
export type SectionScope = WorkspaceId | 'shared';
export type SectionId =
  | 'overview'
  | 'setup'
  | 'guide'
  | 'hall'
  | 'identity'
  | 'vote'
  | 'ops';

export interface SectionMeta {
  id: SectionId;
  workspace: SectionScope;
  title: string;
  description: string;
}

export interface WorkspaceMeta {
  id: WorkspaceId;
  title: string;
  description: string;
}

export interface ProposalItem {
  id: string;
  title: string;
  description: string;
  options: string[];
  status: string;
  startsAt: string;
  endsAt: string;
}

export interface Notice {
  id: string;
  level: 'success' | 'error' | 'info';
  title: string;
  detail: string;
  at: string;
}

export interface AnchorItem {
  id: string;
  anchorHash: string;
  merkleRoot: string;
  createdAt: string;
}

export interface CredentialStatusItem {
  credentialId: string;
  proposalId: string;
  userId: string;
  userPubKey: string;
  issuerKeyVersion: string;
  issuedAt: string;
  hasVoted: boolean;
}

export interface PortalRouteState {
  workspace: WorkspaceId;
  section: SectionId;
  proposalId: string;
}

export type JsonMap = Record<string, unknown>;

export type FormErrorMap = Record<string, string>;

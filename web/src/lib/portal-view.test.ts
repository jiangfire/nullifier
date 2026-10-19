import { describe, expect, it } from 'vitest';
import {
  formatProposalTag,
  OPS_STEPS,
  PAGE_TITLES,
  resolveNextGuideIndex,
  resolveWorkspaceEntry,
  resolveWorkspaceForSection,
} from './portal-view';

describe('portal view helpers', () => {
  it('maps shared sections to no workspace', () => {
    expect(resolveWorkspaceForSection('overview')).toBeNull();
    expect(resolveWorkspaceForSection('setup')).toBeNull();
    expect(resolveWorkspaceForSection('guide')).toBeNull();
  });

  it('maps workflow sections to their workspace', () => {
    expect(resolveWorkspaceForSection('hall')).toBe('user');
    expect(resolveWorkspaceForSection('identity')).toBe('user');
    expect(resolveWorkspaceForSection('vote')).toBe('user');
    expect(resolveWorkspaceForSection('ops')).toBe('admin');
  });

  it('resolves the landing section per workspace', () => {
    expect(resolveWorkspaceEntry('user')).toBe('hall');
    expect(resolveWorkspaceEntry('admin')).toBe('ops');
  });

  it('advances the guide until the last step', () => {
    expect(resolveNextGuideIndex(0, 4)).toBe(1);
    expect(resolveNextGuideIndex(2, 4)).toBe(3);
    expect(resolveNextGuideIndex(3, 4)).toBeNull();
  });

  it('treats an empty guide as finished', () => {
    expect(resolveNextGuideIndex(0, 0)).toBeNull();
  });

  it('formats the proposal tag shown in the top bar', () => {
    expect(formatProposalTag('')).toBe('未选提案');
    expect(formatProposalTag('  ')).toBe('未选提案');
    expect(formatProposalTag('af83005c-9646')).toBe('#af8300');
  });

  it('keeps a title for every section', () => {
    const sections = [
      'overview',
      'setup',
      'guide',
      'hall',
      'identity',
      'vote',
      'ops',
    ] as const;
    for (const section of sections) {
      expect(PAGE_TITLES[section]).toBeTruthy();
    }
  });

  it('keeps the ops steps in a fixed order', () => {
    expect(OPS_STEPS.map((step) => step.id)).toEqual([
      'create',
      'decrypt',
      'publish',
      'result',
      'credential',
      'anchor',
    ]);
  });
});

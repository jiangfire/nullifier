import { describe, expect, it } from 'vitest';
import { buildPortalSearch, parsePortalRoute } from './navigation';

describe('navigation utilities', () => {
  it('parses valid route parameters', () => {
    expect(parsePortalRoute('?ws=admin&tab=ops&proposal=p-001')).toEqual({
      workspace: 'admin',
      section: 'ops',
      proposalId: 'p-001',
    });
  });

  it('defaults blank route to overview', () => {
    expect(parsePortalRoute('')).toEqual({
      workspace: 'user',
      section: 'overview',
      proposalId: '',
    });
  });

  it('normalizes invalid section to workspace default', () => {
    expect(parsePortalRoute('?ws=admin&tab=vote')).toEqual({
      workspace: 'admin',
      section: 'ops',
      proposalId: '',
    });
  });

  it('builds stable query string from state', () => {
    const query = buildPortalSearch({
      workspace: 'user',
      section: 'overview',
      proposalId: 'abc',
    });
    expect(query).toBe('ws=user&tab=overview&proposal=abc');
  });
});

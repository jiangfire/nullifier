import { describe, expect, it } from 'vitest';
import {
  UNBOUND_BALLOT_TARGET,
  hasBallotTargetDrifted,
  isBallotTargetCurrent,
  isTargetUnknown,
} from './ballot';

describe('isTargetUnknown', () => {
  it('没有选项就不可判定', () => {
    expect(isTargetUnknown(UNBOUND_BALLOT_TARGET)).toBe(true);
    expect(isTargetUnknown({ proposalId: 'p1', option: '' })).toBe(true);
  });

  it('有选项就可判定', () => {
    expect(isTargetUnknown({ proposalId: 'p1', option: 'A' })).toBe(false);
  });
});

describe('isBallotTargetCurrent', () => {
  it('目标不可判定时不拦截（外部导入的凭据无法反推选项）', () => {
    expect(
      isBallotTargetCurrent(UNBOUND_BALLOT_TARGET, { proposalId: 'p1', option: 'A' }),
    ).toBe(true);
    expect(
      isBallotTargetCurrent(
        { proposalId: 'p1', option: '' },
        { proposalId: 'p1', option: 'A' },
      ),
    ).toBe(true);
  });

  it('提案与选项都一致时算对得上', () => {
    expect(
      isBallotTargetCurrent(
        { proposalId: 'p1', option: 'A' },
        { proposalId: 'p1', option: 'A' },
      ),
    ).toBe(true);
  });

  it('选项变了就对不上', () => {
    expect(
      isBallotTargetCurrent(
        { proposalId: 'p1', option: 'A' },
        { proposalId: 'p1', option: 'B' },
      ),
    ).toBe(false);
  });

  it('提案变了也对不上', () => {
    expect(
      isBallotTargetCurrent(
        { proposalId: 'p1', option: 'A' },
        { proposalId: 'p2', option: 'A' },
      ),
    ).toBe(false);
  });

  it('有一方没填提案时只比选项', () => {
    expect(
      isBallotTargetCurrent(
        { proposalId: 'p1', option: 'A' },
        { proposalId: '', option: 'A' },
      ),
    ).toBe(true);
  });
});

describe('hasBallotTargetDrifted', () => {
  it('目标不可判定时没有漂移', () => {
    expect(
      hasBallotTargetDrifted(UNBOUND_BALLOT_TARGET, { proposalId: 'p1', option: 'A' }),
    ).toBe(false);
    expect(
      hasBallotTargetDrifted(
        { proposalId: 'p1', option: '' },
        { proposalId: 'p1', option: 'A' },
      ),
    ).toBe(false);
  });

  it('选项变了算漂移', () => {
    expect(
      hasBallotTargetDrifted(
        { proposalId: 'p1', option: 'A' },
        { proposalId: 'p1', option: 'B' },
      ),
    ).toBe(true);
  });

  it('提案变了算漂移', () => {
    expect(
      hasBallotTargetDrifted(
        { proposalId: 'p1', option: 'A' },
        { proposalId: 'p2', option: 'A' },
      ),
    ).toBe(true);
  });

  it('目标一致时没有漂移', () => {
    expect(
      hasBallotTargetDrifted(
        { proposalId: 'p1', option: 'A' },
        { proposalId: 'p1', option: 'A' },
      ),
    ).toBe(false);
  });
});

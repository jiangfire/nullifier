import { describe, expect, it } from 'vitest';
import { localInputToRfc3339, rfc3339ToLocalInput, toLocalDateTimeInput } from './datetime';

describe('datetime utilities', () => {
  it('formats Date to datetime-local string', () => {
    const date = new Date(2026, 2, 10, 16, 8, 0, 0);
    expect(toLocalDateTimeInput(date)).toBe('2026-03-10T16:08');
  });

  it('converts local input to RFC3339 and back', () => {
    const local = '2026-03-10T14:35';
    const rfc = localInputToRfc3339(local);
    expect(rfc).not.toBeNull();
    expect(rfc3339ToLocalInput(rfc ?? '')).toBe(local);
  });

  it('returns null for invalid local input', () => {
    expect(localInputToRfc3339('invalid-date')).toBeNull();
  });
});

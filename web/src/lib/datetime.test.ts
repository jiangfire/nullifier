import { describe, expect, it } from 'vitest';
import {
  formatDateTime,
  formatDateTimeRange,
  localInputToRfc3339,
  rfc3339ToLocalInput,
  toLocalDateTimeInput,
} from './datetime';

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

  it('formats a timestamp for display', () => {
    const date = new Date(2026, 2, 10, 16, 8, 0, 0);
    expect(formatDateTime(date.toISOString())).toBe('2026-03-10 16:08');
  });

  it('falls back to the raw value when the timestamp is unparsable', () => {
    expect(formatDateTime('not-a-date')).toBe('not-a-date');
    expect(formatDateTime('  ')).toBe('-');
  });

  it('joins the voting window into one line', () => {
    const start = new Date(2026, 2, 10, 16, 0, 0, 0);
    const end = new Date(2026, 2, 10, 17, 30, 0, 0);
    expect(formatDateTimeRange(start.toISOString(), end.toISOString())).toBe(
      '2026-03-10 16:00 → 2026-03-10 17:30',
    );
  });

  it('returns a dash when both ends of the window are missing', () => {
    expect(formatDateTimeRange('', '')).toBe('-');
  });
});

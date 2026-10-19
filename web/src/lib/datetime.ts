function pad(value: number): string {
  return String(value).padStart(2, '0');
}

export function toRfc3339(date: Date): string {
  return date.toISOString().replace(/\.\d{3}Z$/, 'Z');
}

export function toLocalDateTimeInput(date: Date): string {
  return [
    date.getFullYear(),
    '-',
    pad(date.getMonth() + 1),
    '-',
    pad(date.getDate()),
    'T',
    pad(date.getHours()),
    ':',
    pad(date.getMinutes()),
  ].join('');
}

export function rfc3339ToLocalInput(value: string): string {
  const text = value.trim();
  if (text === '') {
    return '';
  }
  const parsed = new Date(text);
  if (Number.isNaN(parsed.getTime())) {
    return '';
  }
  return toLocalDateTimeInput(parsed);
}

/** 把后端返回的时间戳格式化成紧凑的本地时间；无法解析时原样返回。 */
export function formatDateTime(value: string): string {
  const text = value.trim();
  if (text === '') {
    return '-';
  }
  const parsed = new Date(text);
  if (Number.isNaN(parsed.getTime())) {
    return text;
  }
  return [
    parsed.getFullYear(),
    '-',
    pad(parsed.getMonth() + 1),
    '-',
    pad(parsed.getDate()),
    ' ',
    pad(parsed.getHours()),
    ':',
    pad(parsed.getMinutes()),
  ].join('');
}

/** 投票窗口：起止时间拼成一行，两端都缺失时返回 '-'。 */
export function formatDateTimeRange(start: string, end: string): string {
  const from = formatDateTime(start);
  const to = formatDateTime(end);
  if (from === '-' && to === '-') {
    return '-';
  }
  return `${from} → ${to}`;
}

export function localInputToRfc3339(value: string): string | null {
  const text = value.trim();
  if (text === '') {
    return null;
  }
  const parsed = new Date(text);
  if (Number.isNaN(parsed.getTime())) {
    return null;
  }
  return toRfc3339(parsed);
}

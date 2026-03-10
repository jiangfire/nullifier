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

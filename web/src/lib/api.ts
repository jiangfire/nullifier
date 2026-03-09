export type ApiMethod = 'GET' | 'POST';

export type QueryValue = string | number | boolean | null | undefined;

export interface ApiCallOptions {
  baseUrl: string;
  path: string;
  method?: ApiMethod;
  query?: Record<string, QueryValue>;
  body?: unknown;
  signal?: AbortSignal;
}

export interface ApiCallResult<T = unknown> {
  ok: boolean;
  status: number;
  data: T | null;
  error: string | null;
  rawText: string;
}

function normalizeBaseUrl(baseUrl: string): string {
  const trimmed = baseUrl.trim();
  if (trimmed.length === 0) {
    return '';
  }
  return trimmed.endsWith('/') ? trimmed.slice(0, -1) : trimmed;
}

function normalizePath(path: string): string {
  if (path.startsWith('/')) {
    return path;
  }
  return `/${path}`;
}

function buildUrl(baseUrl: string, path: string, query?: Record<string, QueryValue>): string {
  const normalizedBase = normalizeBaseUrl(baseUrl);
  const normalizedPath = normalizePath(path);
  const effectiveBase = normalizedBase.length > 0 ? normalizedBase : window.location.origin;
  const url = new URL(`${effectiveBase}${normalizedPath}`);

  if (query) {
    for (const [key, value] of Object.entries(query)) {
      if (value === undefined || value === null || value === '') {
        continue;
      }
      url.searchParams.set(key, String(value));
    }
  }

  return url.toString();
}

function extractError(payload: unknown, status: number): string {
  if (typeof payload === 'object' && payload !== null) {
    const candidate = payload as Record<string, unknown>;
    if (typeof candidate.error === 'string' && candidate.error.trim().length > 0) {
      return candidate.error;
    }
    if (typeof candidate.message === 'string' && candidate.message.trim().length > 0) {
      return candidate.message;
    }
  }
  return `请求失败（HTTP ${status}）`;
}

export async function apiCall<T = unknown>(options: ApiCallOptions): Promise<ApiCallResult<T>> {
  const { baseUrl, path, method = 'GET', query, body, signal } = options;
  const url = buildUrl(baseUrl, path, query);

  const headers = new Headers();
  const requestInit: RequestInit = { method, headers, signal };
  if (body !== undefined) {
    headers.set('Content-Type', 'application/json');
    requestInit.body = JSON.stringify(body);
  }

  try {
    const response = await fetch(url, requestInit);
    const rawText = await response.text();
    let parsed: unknown = null;
    if (rawText.trim().length > 0) {
      try {
        parsed = JSON.parse(rawText);
      } catch {
        parsed = rawText;
      }
    }

    if (!response.ok) {
      return {
        ok: false,
        status: response.status,
        data: null,
        error: extractError(parsed, response.status),
        rawText,
      };
    }

    return {
      ok: true,
      status: response.status,
      data: (parsed as T) ?? null,
      error: null,
      rawText,
    };
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    return {
      ok: false,
      status: 0,
      data: null,
      error: `网络请求失败：${message}`,
      rawText: '',
    };
  }
}

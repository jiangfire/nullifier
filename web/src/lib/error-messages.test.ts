import { describe, expect, it } from 'vitest';
import { CODE_MESSAGES, describeApiError } from './error-messages';

/**
 * 门户认识的全部后端错误码。
 *
 * 这份清单与 `internal/infrastructure/http/middleware/error_handler.go` 的
 * `MapError` **必须一致** —— 跨语言门禁
 * （`tests/architecture/error_code_test.go`）会比对两侧。
 * 服务端加一个码而门户不认识，用户看到的就是一句英文原文。
 */
const KNOWN_ERROR_CODES = [
  'INVALID_REQUEST',
  'DOUBLE_VOTING',
  'CREDENTIAL_EXISTS',
  'USER_ALREADY_VOTED',
  'INVALID_CREDENTIAL',
  'DECRYPTION_SESSION_EXISTS',
  'RESULT_ALREADY_PUBLISHED',
  'UNAUTHORIZED',
  'NOT_FOUND',
  'ELIGIBILITY_REQUIRED',
  'INVALID_ELIGIBILITY_PROOF',
  'ELIGIBILITY_ALREADY_USED',
  'ELIGIBILITY_NOT_ENABLED',
  'CREDENTIAL_POOL_EMPTY',
  'RATE_LIMITED',
  'INTERNAL_ERROR',
];

describe('error code contract', () => {
  it('knows every backend error code', () => {
    expect(Object.keys(CODE_MESSAGES).sort()).toEqual([...KNOWN_ERROR_CODES].sort());
  });

  it('gives every error code a Chinese message', () => {
    for (const code of KNOWN_ERROR_CODES) {
      const message = CODE_MESSAGES[code as keyof typeof CODE_MESSAGES];
      expect(message, `${code} 缺少中文文案`).toBeTruthy();
      expect(message, `${code} 的文案不能是英文原文`).toMatch(/[一-鿿]/);
    }
  });
});

describe('describeApiError', () => {
  it('translates the decryption-window business rule', () => {
    const message =
      'invalid request: proposal decryption must be prepared before voting opens (proposal_id=p-1 status=open now=... starts_at=... ends_at=...)';
    const detail = describeApiError(message, 'INVALID_REQUEST', '兜底');
    expect(detail).toContain('该提案已进入投票窗口');
    expect(detail).not.toContain('invalid request');
  });

  it('translates known request details', () => {
    expect(
      describeApiError(
        'invalid request: proposal_id is required',
        'INVALID_REQUEST',
        '兜底',
      ),
    ).toBe('请先选择提案（proposal_id）。');
    expect(
      describeApiError(
        'invalid request: ends_at must be after starts_at',
        'INVALID_REQUEST',
        '兜底',
      ),
    ).toBe('结束时间必须晚于开始时间。');
  });

  it('prefers the call-site context for contextual codes', () => {
    expect(
      describeApiError(
        'resource not found',
        'NOT_FOUND',
        '该提案还没初始化解密，请先完成「初始化解密」',
      ),
    ).toBe('该提案还没初始化解密，请先完成「初始化解密」');
  });

  it('translates the tally-window business rules', () => {
    expect(
      describeApiError(
        'invalid request: proposal must be closed before tally',
        'INVALID_REQUEST',
        '兜底',
      ),
    ).toBe('该提案的投票窗口还没结束。等 ends_at 之后再发布结果。');
    expect(
      describeApiError(
        'invalid request: proposal is not open for voting',
        'INVALID_REQUEST',
        '兜底',
      ),
    ).toBe('投票窗口未开启或已经结束，现在不能投票。');
    expect(
      describeApiError(
        'invalid request: decryption session is not ready',
        'INVALID_REQUEST',
        '兜底',
      ),
    ).toBe('该提案还没有初始化解密，请先在「计票与审计」完成「初始化解密」。');
  });

  it('keeps the backend detail when no phrase is known', () => {
    expect(
      describeApiError(
        'invalid request: something the backend invented',
        'INVALID_REQUEST',
        '兜底',
      ),
    ).toBe('something the backend invented');
  });

  it('uses the code message for systemic codes', () => {
    expect(describeApiError('unauthorized', 'UNAUTHORIZED', '兜底')).toBe(
      'Admin Token 缺失或无效，请在「连接配置」里填好',
    );
    expect(describeApiError('double voting', 'DOUBLE_VOTING', '兜底')).toBe(
      '这张凭证已经投过票，不能重复提交',
    );
    expect(
      describeApiError('internal server error', 'INTERNAL_ERROR', '兜底'),
    ).toBe('服务端出错，请查看后端日志');
  });

  it('translates the rate-limit code', () => {
    expect(describeApiError('rate limit exceeded', 'RATE_LIMITED', '兜底')).toBe(
      '请求太频繁了，请稍等片刻再试',
    );
  });

  it('translates network failures', () => {
    expect(
      describeApiError('网络请求失败：Failed to fetch', null, '兜底'),
    ).toBe('无法连接后端服务，请检查地址与网络');
  });

  it('falls back to the original text for unknown errors', () => {
    expect(describeApiError('something odd happened', null, '兜底')).toBe(
      'something odd happened',
    );
  });

  it('falls back to the caller text when there is no error at all', () => {
    expect(describeApiError(null, null, '兜底')).toBe('兜底');
    expect(describeApiError('   ', null, '兜底')).toBe('兜底');
  });
});

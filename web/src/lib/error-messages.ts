/**
 * 后端错误码 → 中文提示。
 *
 * 后端 `ErrorHandler` 会返回 `{ error, code }`，其中 `code` 是稳定枚举，
 * `error` 是面向开发者的英文（部分场景会带上原始 Go error 细节）。
 * 这里按「业务短语 > 错误码 > 调用方兜底」的优先级翻译成人话。
 */

/** 与 `internal/infrastructure/http/middleware/error_handler.go` 的错误码保持一致。 */
export type PortalErrorCode =
  | 'INVALID_REQUEST'
  | 'DOUBLE_VOTING'
  | 'CREDENTIAL_EXISTS'
  | 'USER_ALREADY_VOTED'
  | 'INVALID_CREDENTIAL'
  | 'DECRYPTION_SESSION_EXISTS'
  | 'RESULT_ALREADY_PUBLISHED'
  | 'UNAUTHORIZED'
  | 'NOT_FOUND'
  | 'ELIGIBILITY_REQUIRED'
  | 'INVALID_ELIGIBILITY_PROOF'
  | 'ELIGIBILITY_ALREADY_USED'
  | 'ELIGIBILITY_NOT_ENABLED'
  | 'CREDENTIAL_POOL_EMPTY'
  | 'RATE_LIMITED'
  | 'INTERNAL_ERROR';

/**
 * 与具体操作无关的失败：直接用错误码的通用说法。
 *
 * 导出是为了让测试能钉住这张表 —— 错误码是**跨语言契约**的一部分，
 * 服务端加一个码而门户不认识，用户看到的就是一句英文原文。
 */
export const CODE_MESSAGES: Record<PortalErrorCode, string> = {
  INVALID_REQUEST: '提交的内容不符合要求',
  DOUBLE_VOTING: '这张凭证已经投过票，不能重复提交',
  CREDENTIAL_EXISTS: '该用户已领取过本提案的凭证',
  USER_ALREADY_VOTED: '你已经投过这一票',
  INVALID_CREDENTIAL: '凭证无效，请重新领取或导入',
  DECRYPTION_SESSION_EXISTS: '该提案已初始化过解密会话，无需重复初始化',
  RESULT_ALREADY_PUBLISHED: '结果已发布，无需重复发布',
  UNAUTHORIZED: 'Admin Token 缺失或无效，请在「连接配置」里填好',
  NOT_FOUND: '需要的数据还不存在',
  ELIGIBILITY_REQUIRED: '服务端要求先证明你有投票资格，请导入你的资格文件再领凭证',
  INVALID_ELIGIBILITY_PROOF: '资格证明没通过校验，请确认资格文件与名单、提案是否对应',
  ELIGIBILITY_ALREADY_USED: '这份资格已经领过票了 —— 一人一票',
  ELIGIBILITY_NOT_ENABLED: '服务端没有开启资格校验（一人一票）',
  CREDENTIAL_POOL_EMPTY: '本提案的凭证池已经领完了',
  RATE_LIMITED: '请求太频繁了，请稍等片刻再试',
  INTERNAL_ERROR: '服务端出错，请查看后端日志',
};

/**
 * 与操作强相关的失败：优先用调用方给的上下文提示，
 * 否则回落到错误码的通用说法。
 */
const CONTEXTUAL_CODES: ReadonlySet<string> = new Set([
  'INVALID_REQUEST',
  'NOT_FOUND',
]);
/**
 * 后端透传的业务细节，按短语翻译。命中第一条即返回。
 * 顺序即优先级：越具体的规则越靠前。
 */
const MESSAGE_RULES: Array<{ match: RegExp; text: string }> = [
  {
    match: /proposal decryption must be prepared before voting opens/i,
    text: '该提案已进入投票窗口，而解密必须在开放投票之前完成。请把 starts_at 设为未来时间后重试，或新建一场「待开始」的提案。',
  },
  {
    match: /proposal must be closed before tally/i,
    text: '该提案的投票窗口还没结束。等 ends_at 之后再发布结果。',
  },
  {
    match: /proposal is not open for voting/i,
    text: '投票窗口未开启或已经结束，现在不能投票。',
  },
  {
    match: /decryption session is not ready/i,
    text: '该提案还没有初始化解密，请先在「计票与审计」完成「初始化解密」。',
  },
  {
    match: /share does not belong to session/i,
    text: '提交的解密份额不属于这个提案的解密会话。',
  },
  { match: /duplicate share/i, text: '同一份解密份额不能重复提交。' },
  {
    match: /insufficient shares/i,
    text: '提交的解密份额数量不够，达不到 threshold。',
  },
  {
    match: /invalid share format/i,
    text: '解密份额格式不正确，请重新初始化解密会话。',
  },
  {
    match: /invalid encrypted vote/i,
    text: '加密选票格式不正确，请重新生成投票凭据。',
  },
  { match: /empty candidate/i, text: '存在空选项的选票，无法计票。' },
  {
    match: /is not defined for proposal/i,
    text: '该选项不属于当前提案，请重新选择。',
  },
  { match: /duplicate option/i, text: '候选项不能重复。' },
  { match: /option cannot be empty/i, text: '候选项不能为空。' },
  { match: /at least 2 options are required/i, text: '至少需要两个候选项。' },
  { match: /title is required/i, text: '请填写提案标题。' },
  { match: /starts_at is required/i, text: '请填写投票开始时间。' },
  { match: /ends_at is required/i, text: '请填写投票结束时间。' },
  {
    match: /ends_at must be after starts_at/i,
    text: '结束时间必须晚于开始时间。',
  },
  { match: /proposal_id is required/i, text: '请先选择提案（proposal_id）。' },
  { match: /user_id is required/i, text: '请填写 user_id。' },
  { match: /user_pub_key is required/i, text: '请填写设备公钥。' },
  {
    match: /credential_signature is required/i,
    text: '凭证签名缺失，请重新领取凭证。',
  },
  {
    match: /encrypted_vote is required/i,
    text: '缺少加密选票，请先生成投票凭据。',
  },
  { match: /proof is required/i, text: '缺少 proof，请先生成投票凭据。' },
  {
    match: /nullifier is required/i,
    text: '缺少 nullifier，请先生成投票凭据。',
  },
  { match: /option is required/i, text: '请先选择投票选项。' },
  { match: /shares are required/i, text: '缺少解密份额（shares）。' },
  {
    match: /trustees must be >= threshold/i,
    text: 'trustees（总份额数）必须大于等于 threshold。',
  },
  {
    match: /threshold must be positive/i,
    text: 'threshold（门限份额数）必须大于 0。',
  },
  { match: /threshold/i, text: 'threshold（门限份额数）不合法。' },
  { match: /trustees/i, text: 'trustees（总份额数）不合法。' },
  { match: /request is nil/i, text: '请求内容为空。' },
];

function isPortalErrorCode(code: string | null): code is PortalErrorCode {
  return code !== null && code in CODE_MESSAGES;
}

/**
 * 把一次失败的 API 调用翻译成可直接展示的中文。
 *
 * @param message 后端返回的原始错误文本（可能是英文或原始 Go error）
 * @param code    后端错误码
 * @param fallback 调用方给出的上下文兜底文案
 */
export function describeApiError(
  message: string | null,
  code: string | null,
  fallback: string,
): string {
  const text = (message ?? '').trim();

  for (const rule of MESSAGE_RULES) {
    if (rule.match.test(text)) {
      return rule.text;
    }
  }

  // 资源不存在：调用方知道具体缺的是哪一样，用它的上下文文案。
  if (code === 'NOT_FOUND') {
    return fallback.trim().length > 0 ? fallback : CODE_MESSAGES.NOT_FOUND;
  }

  if (isPortalErrorCode(code)) {
    if (code === 'INVALID_REQUEST') {
      // 没命中已知短语时，去掉前缀把后端细节原样透出，好过含糊的兜底文案。
      if (text.length > 0) {
        return text.replace(/^invalid request:\s*/i, '');
      }
      return fallback;
    }
    if (!CONTEXTUAL_CODES.has(code)) {
      return CODE_MESSAGES[code];
    }
    return fallback.trim().length > 0 ? fallback : CODE_MESSAGES[code];
  }

  if (text.startsWith('网络请求失败')) {
    return '无法连接后端服务，请检查地址与网络';
  }

  if (text.length > 0) {
    return text;
  }

  return fallback;
}

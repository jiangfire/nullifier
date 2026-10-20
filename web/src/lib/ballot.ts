/**
 * 投票凭据材料与「当前意图」的绑定规则。
 *
 * 背景：凭据（密文 + proof）是针对**某一个选项**生成的。用户在生成之后改了
 * 选项，服务端既不消费 `selected_option`、也无法从密文反推选项，于是会记下
 * 旧选项 —— 用户看到的是 B，实际计的是 A，而且没有任何提示。
 *
 * 这个模块把「材料是否还对得上当前意图」这条规则单独拎出来，便于单测；
 * composable 负责把它接到界面状态上。
 */

/** 凭据材料所绑定的目标。 */
export interface BallotTarget {
  proposalId: string;
  option: string;
}

/** 尚未绑定任何目标（还没生成过，或已作废）。 */
export const UNBOUND_BALLOT_TARGET: BallotTarget = Object.freeze({
  proposalId: '',
  option: '',
});

/**
 * 目标是否「不可判定」：没有选项就无从判断材料对不对。
 *
 * 外部导入的凭据可能不带 `selected_option`，服务端加密的模式下我们也无法从
 * 密文反推选项 —— 这种材料只能交给用户自己核对，不该被拦住。
 */
export function isTargetUnknown(target: BallotTarget): boolean {
  return target.option === '';
}

/** 提案是否明确冲突（有一方没填提案时不算冲突）。 */
function hasProposalConflict(bound: BallotTarget, current: BallotTarget): boolean {
  if (bound.proposalId === '' || current.proposalId === '') {
    return false;
  }
  return bound.proposalId !== current.proposalId;
}

/**
 * 材料是否仍然对应当前意图。
 *
 * 目标不可判定时返回 true（不拦），理由见 {@link isTargetUnknown}。
 */
export function isBallotTargetCurrent(
  bound: BallotTarget,
  current: BallotTarget,
): boolean {
  if (isTargetUnknown(bound)) {
    return true;
  }
  if (hasProposalConflict(bound, current)) {
    return false;
  }
  return bound.option === current.option;
}

/**
 * 当前意图是否已经偏离材料绑定的目标。
 * 偏离了就要作废旧材料，否则会投出旧选项。
 */
export function hasBallotTargetDrifted(
  bound: BallotTarget,
  current: BallotTarget,
): boolean {
  if (isTargetUnknown(bound)) {
    return false;
  }
  if (hasProposalConflict(bound, current)) {
    return true;
  }
  return bound.option !== current.option;
}

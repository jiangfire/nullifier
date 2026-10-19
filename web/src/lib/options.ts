/**
 * 提案候选项的行式编辑规则。
 *
 * 候选项在界面上是「一行一个输入框 + 增删按钮」，不是一段按行分割的文本，
 * 所以这里的函数都只处理「选项行数组」这一种形状。
 */

/** 后端要求至少两个候选项，界面上也至少保留两行，避免删到无法提交。 */
export const MIN_OPTION_ROWS = 2;

/** 去掉首尾空格、丢掉空行、去掉重复项，得到真正要提交的候选项。 */
export function normalizeOptionList(list: readonly string[]): string[] {
  return list
    .map((item) => item.trim())
    .filter((item) => item.length > 0)
    .filter((item, index, array) => array.indexOf(item) === index);
}

/** 当前行数下还能不能继续删除。 */
export function canRemoveOptionRow(list: readonly string[]): boolean {
  return list.length > MIN_OPTION_ROWS;
}

/** 追加一个空行，返回新数组，不修改入参。 */
export function appendOptionRow(list: readonly string[]): string[] {
  return [...list, ''];
}

/** 删除指定行；行数已到下限时原样返回。 */
export function removeOptionRow(
  list: readonly string[],
  index: number,
): string[] {
  if (!canRemoveOptionRow(list)) {
    return [...list];
  }
  if (index < 0 || index >= list.length) {
    return [...list];
  }
  const next = [...list];
  next.splice(index, 1);
  return next;
}

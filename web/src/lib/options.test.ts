import { describe, expect, it } from 'vitest';
import {
  MIN_OPTION_ROWS,
  appendOptionRow,
  canRemoveOptionRow,
  normalizeOptionList,
  removeOptionRow,
} from './options';

describe('normalizeOptionList', () => {
  it('去掉首尾空格与空行', () => {
    expect(normalizeOptionList(['  方案甲  ', '', '   ', '方案乙'])).toEqual([
      '方案甲',
      '方案乙',
    ]);
  });

  it('去掉重复项并保留首次出现的顺序', () => {
    expect(normalizeOptionList(['方案甲', '方案乙', '方案甲'])).toEqual([
      '方案甲',
      '方案乙',
    ]);
  });

  it('全是空行时返回空数组', () => {
    expect(normalizeOptionList(['', '  '])).toEqual([]);
  });
});

describe('canRemoveOptionRow', () => {
  it('到下限行数时不能再删', () => {
    expect(canRemoveOptionRow(['a', 'b'])).toBe(false);
    expect(MIN_OPTION_ROWS).toBe(2);
  });

  it('超过下限行数时可以删', () => {
    expect(canRemoveOptionRow(['a', 'b', 'c'])).toBe(true);
  });
});

describe('appendOptionRow', () => {
  it('追加一个空行且不修改入参', () => {
    const rows = ['方案甲', '方案乙'];
    expect(appendOptionRow(rows)).toEqual(['方案甲', '方案乙', '']);
    expect(rows).toEqual(['方案甲', '方案乙']);
  });
});

describe('removeOptionRow', () => {
  it('删除指定行', () => {
    expect(removeOptionRow(['a', 'b', 'c'], 1)).toEqual(['a', 'c']);
  });

  it('到下限行数时拒绝删除', () => {
    expect(removeOptionRow(['a', 'b'], 0)).toEqual(['a', 'b']);
  });

  it('越界下标原样返回', () => {
    expect(removeOptionRow(['a', 'b', 'c'], 9)).toEqual(['a', 'b', 'c']);
    expect(removeOptionRow(['a', 'b', 'c'], -1)).toEqual(['a', 'b', 'c']);
  });
});

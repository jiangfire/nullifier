export interface FocusableElement {
  focus(): void;
}

export interface FocusTrapContainer {
  querySelectorAll(selector: string): ArrayLike<FocusableElement>;
  contains(node: unknown): boolean;
}

export interface FocusTrapDocument {
  activeElement: unknown;
}

const FOCUSABLE_SELECTOR = [
  'a[href]',
  'button:not([disabled])',
  'input:not([disabled])',
  'select:not([disabled])',
  'textarea:not([disabled])',
  '[tabindex]:not([tabindex="-1"])',
].join(', ');

/**
 * 模态浮层的焦点陷阱。
 *
 * 打开时把焦点移进浮层，Tab / Shift+Tab 在浮层内循环，关闭时把焦点还给
 * 打开它的那个元素 —— 否则键盘用户会被留在浮层后面的页面里，既看不到
 * 浮层内容，也回不去。
 */
export class FocusTrap {
  private readonly container: FocusTrapContainer;
  private readonly doc: FocusTrapDocument;
  private previous: FocusableElement | null = null;

  constructor(container: FocusTrapContainer, doc: FocusTrapDocument) {
    this.container = container;
    this.doc = doc;
  }

  /** 浮层里可聚焦的元素。 */
  items(): FocusableElement[] {
    return Array.from(this.container.querySelectorAll(FOCUSABLE_SELECTOR));
  }

  /** 记录当前焦点，并聚焦浮层里的第一个可聚焦元素。 */
  activate(): void {
    const active = this.doc.activeElement as FocusableElement | null;
    this.previous = active && typeof active.focus === 'function' ? active : null;
    this.items()[0]?.focus();
  }

  /** 把焦点还给打开浮层的那个元素。 */
  deactivate(): void {
    this.previous?.focus();
    this.previous = null;
  }

  /**
   * 处理 Tab / Shift+Tab。
   *
   * 焦点在最后一个元素上按 Tab 时回到第一个；在第一个元素上按 Shift+Tab
   * 时跳到最后一个；焦点在浮层之外时直接拉回第一个。
   *
   * 返回 true 表示这次按键已经被处理了。
   */
  handleTab(shiftKey: boolean): boolean {
    const items = this.items();
    if (items.length === 0) {
      return false;
    }

    const active = this.doc.activeElement;
    const inside = this.container.contains(active);

    if (shiftKey) {
      if (!inside || active === items[0]) {
        items[items.length - 1].focus();
        return true;
      }
      return false;
    }

    if (!inside || active === items[items.length - 1]) {
      items[0].focus();
      return true;
    }
    return false;
  }
}

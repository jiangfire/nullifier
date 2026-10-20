import { describe, expect, it } from 'vitest';
import {
  FocusTrap,
  type FocusTrapContainer,
  type FocusableElement,
} from './focus-trap';

class FakeElement implements FocusableElement {
  focused = false;

  focus(): void {
    this.focused = true;
  }
}

class FakeContainer implements FocusTrapContainer {
  constructor(readonly elements: FakeElement[]) {}

  querySelectorAll(): ArrayLike<FocusableElement> {
    return this.elements;
  }

  contains(node: unknown): boolean {
    return this.elements.includes(node as FakeElement);
  }
}

class FakeDocument {
  activeElement: unknown = null;
}

function setup(): {
  trap: FocusTrap;
  doc: FakeDocument;
  first: FakeElement;
  middle: FakeElement;
  last: FakeElement;
  outside: FakeElement;
} {
  const first = new FakeElement();
  const middle = new FakeElement();
  const last = new FakeElement();
  const outside = new FakeElement();
  const doc = new FakeDocument();
  const container = new FakeContainer([first, middle, last]);
  return {
    trap: new FocusTrap(container, doc),
    doc,
    first,
    middle,
    last,
    outside,
  };
}

function clearFocus(elements: FakeElement[]): void {
  for (const element of elements) {
    element.focused = false;
  }
}

describe('浮层焦点陷阱', () => {
  it('打开时把焦点移进浮层的第一个元素', () => {
    const { trap, first } = setup();

    trap.activate();

    expect(first.focused).toBe(true);
  });

  it('关闭时把焦点还给打开浮层的那个元素', () => {
    const { trap, doc, outside } = setup();
    doc.activeElement = outside;

    trap.activate();
    trap.deactivate();

    expect(outside.focused).toBe(true);
  });

  it('焦点在最后一个元素上按 Tab 时回到第一个', () => {
    const { trap, doc, first, last } = setup();
    trap.activate();
    clearFocus([first, last]);
    doc.activeElement = last;

    expect(trap.handleTab(false)).toBe(true);
    expect(first.focused).toBe(true);
  });

  it('焦点在第一个元素上按 Shift+Tab 时跳到最后一个', () => {
    const { trap, doc, first, last } = setup();
    trap.activate();
    clearFocus([first, last]);
    doc.activeElement = first;

    expect(trap.handleTab(true)).toBe(true);
    expect(last.focused).toBe(true);
  });

  it('焦点在中间元素上时 Tab 不被拦截', () => {
    const { trap, doc, middle, first, last } = setup();
    trap.activate();
    clearFocus([first, middle, last]);
    doc.activeElement = middle;

    expect(trap.handleTab(false)).toBe(false);
    expect(first.focused).toBe(false);
    expect(last.focused).toBe(false);
  });

  it('焦点跑到浮层外面时 Tab 把它拉回来', () => {
    const { trap, doc, outside, first } = setup();
    trap.activate();
    clearFocus([first]);
    doc.activeElement = outside;

    expect(trap.handleTab(false)).toBe(true);
    expect(first.focused).toBe(true);
  });

  it('浮层里没有可聚焦元素时不拦截按键', () => {
    const doc = new FakeDocument();
    const trap = new FocusTrap(new FakeContainer([]), doc);

    expect(trap.handleTab(false)).toBe(false);
    expect(trap.handleTab(true)).toBe(false);
  });
});

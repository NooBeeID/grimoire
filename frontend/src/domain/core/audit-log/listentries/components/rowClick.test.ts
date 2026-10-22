import { describe, expect, it } from 'vitest';
import { shouldOpenFromRowClick } from './rowClick';

function row() {
  const tr = document.createElement('tr');
  tr.innerHTML = '<td><button>t</button></td><td><span>teks</span></td>';
  document.body.append(tr);
  return tr;
}

describe('shouldOpenFromRowClick', () => {
  it('opens on a plain cell click', () => {
    const tr = row();
    expect(shouldOpenFromRowClick({ target: tr.querySelector('span'), currentTarget: tr })).toBe(true);
  });
  it('ignores clicks on controls', () => {
    const tr = row();
    expect(shouldOpenFromRowClick({ target: tr.querySelector('button'), currentTarget: tr })).toBe(false);
  });
  it('ignores a click that ends a text selection inside the row', () => {
    const tr = row();
    const span = tr.querySelector('span')!;
    const range = document.createRange();
    range.selectNodeContents(span);
    window.getSelection()!.removeAllRanges();
    window.getSelection()!.addRange(range);
    expect(shouldOpenFromRowClick({ target: span, currentTarget: tr })).toBe(false);
    window.getSelection()!.removeAllRanges();
  });
});

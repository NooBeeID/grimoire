import { describe, expect, it } from 'vitest';
import { diffLines } from './diff';

describe('diffLines', () => {
  it('marks identical text as all equal', () => {
    expect(diffLines('a\nb', 'a\nb')).toEqual([
      { op: 'eq', text: 'a' },
      { op: 'eq', text: 'b' },
    ]);
  });
  it('marks an added line', () => {
    expect(diffLines('a\nc', 'a\nb\nc')).toEqual([
      { op: 'eq', text: 'a' },
      { op: 'add', text: 'b' },
      { op: 'eq', text: 'c' },
    ]);
  });
  it('marks a removed line', () => {
    expect(diffLines('a\nb\nc', 'a\nc')).toEqual([
      { op: 'eq', text: 'a' },
      { op: 'del', text: 'b' },
      { op: 'eq', text: 'c' },
    ]);
  });
  it('shows a replaced line as delete then add', () => {
    expect(diffLines('a\nold\nc', 'a\nnew\nc')).toEqual([
      { op: 'eq', text: 'a' },
      { op: 'del', text: 'old' },
      { op: 'add', text: 'new' },
      { op: 'eq', text: 'c' },
    ]);
  });
  it('handles completely different text', () => {
    const rows = diffLines('x\ny', 'p\nq')!;
    expect(rows.filter((r) => r.op === 'del')).toHaveLength(2);
    expect(rows.filter((r) => r.op === 'add')).toHaveLength(2);
  });
  it('returns null when either side exceeds 5000 lines', () => {
    const big = Array.from({ length: 5001 }, (_, i) => `l${i}`).join('\n');
    expect(diffLines(big, 'a')).toBeNull();
    expect(diffLines('a', big)).toBeNull();
  });
  it('stays cheap for a single change inside a large file', () => {
    const lines = Array.from({ length: 4000 }, (_, i) => `baris ${i}`);
    const changed = [...lines];
    changed[2000] = 'berubah';
    const rows = diffLines(lines.join('\n'), changed.join('\n'))!;
    expect(rows.filter((r) => r.op !== 'eq')).toEqual([
      { op: 'del', text: 'baris 2000' },
      { op: 'add', text: 'berubah' },
    ]);
  });
});

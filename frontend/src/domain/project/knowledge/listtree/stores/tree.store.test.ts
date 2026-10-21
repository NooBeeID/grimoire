import { get } from 'svelte/store';
import { describe, expect, it } from 'vitest';
import { authLevel, docsLevel, emptyRoot, rootLevel } from '../mocks/tree';
import { createTreeStore } from './tree.store';

describe('tree store', () => {
  it('rows: folders first then files, with depth', () => {
    const s = createTreeStore();
    s.resolve('', rootLevel);
    s.resolve('docs', docsLevel);
    s.expand(['docs']);
    const rows = get(s.rows).map((r) => `${r.depth}:${r.kind}:${r.path}`);
    expect(rows).toEqual([
      '0:folder:docs',
      '1:folder:docs/auth',
      '1:file:docs/arsitektur.md',
      '1:file:docs/catatan.txt',
      '0:file:README.md',
    ]);
  });
  it('toggle reports open state and caches levels', () => {
    const s = createTreeStore();
    s.resolve('', rootLevel);
    expect(s.toggle('docs')).toBe(true);
    s.resolve('docs', docsLevel);
    expect(s.toggle('docs')).toBe(false);
    expect(s.toggle('docs')).toBe(true);
    expect(s.has('docs')).toBe(true);
  });
  it('loading and error rows appear under expanded folder', () => {
    const s = createTreeStore();
    s.resolve('', rootLevel);
    s.expand(['docs']);
    s.beginLoad('docs');
    expect(get(s.rows).some((r) => r.kind === 'loading')).toBe(true);
    s.fail('docs', { status: 500 } as never);
    expect(get(s.rows).some((r) => r.kind === 'error')).toBe(true);
  });
  it('empty folder row', () => {
    const s = createTreeStore();
    s.resolve('', rootLevel);
    s.expand(['docs']);
    s.resolve('docs', { ...authLevel, prefix: 'docs', folders: [], files: [] });
    expect(get(s.rows).some((r) => r.kind === 'empty')).toBe(true);
  });
  it('invalidateAll returns root + expanded and drops other levels', () => {
    const s = createTreeStore();
    s.resolve('', rootLevel);
    s.resolve('docs', docsLevel);
    s.resolve('docs/auth', authLevel);
    s.expand(['docs']);
    expect(s.invalidateAll().sort()).toEqual(['', 'docs']);
    expect(s.has('docs/auth')).toBe(false);
    expect(emptyRoot.total_files).toBe(0);
  });
});

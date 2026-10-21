import { afterEach, describe, expect, it, vi } from 'vitest';
import { clearDraft, loadDraft, saveDraft } from './draft';

afterEach(() => {
  vi.restoreAllMocks();
  sessionStorage.clear();
});
const d = { content: 'x', baseVersion: 'a'.repeat(64), savedAt: '2026-10-21T09:30:00Z' };

describe('draft', () => {
  it('save, load, clear', () => {
    saveDraft('p1', 'a.md', d);
    expect(loadDraft('p1', 'a.md')).toEqual(d);
    clearDraft('p1', 'a.md');
    expect(loadDraft('p1', 'a.md')).toBeNull();
  });
  it('survives storage that throws', () => {
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('quota');
    });
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('denied');
    });
    expect(() => saveDraft('p1', 'a.md', d)).not.toThrow();
    expect(loadDraft('p1', 'a.md')).toBeNull();
  });
  it('corrupt JSON -> null', () => {
    sessionStorage.setItem('grimoire.knowledge.draft:p1:a.md', '{oops');
    expect(loadDraft('p1', 'a.md')).toBeNull();
  });
});

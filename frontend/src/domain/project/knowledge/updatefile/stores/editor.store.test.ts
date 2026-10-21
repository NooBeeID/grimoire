import { get } from 'svelte/store';
import { describe, expect, it } from 'vitest';
import { mockFile, V2 } from '../../base';
import { createEditorStore } from './editor.store';

const current = { content: 'baru', version: V2, updated_at: '2026-10-21T09:30:00Z', updated_by_label: 'budi@tim.id' };

describe('editor store', () => {
  it('dirty follows draft vs base', () => {
    const s = createEditorStore(mockFile('a.md', { content: 'a' }));
    expect(get(s.dirty)).toBe(false);
    s.edit('b');
    expect(get(s.dirty)).toBe(true);
  });
  it('saved updates base and version', () => {
    const s = createEditorStore(mockFile('a.md', { content: 'a' }));
    s.edit('b');
    s.saved(mockFile('a.md', { content: 'b', version: V2 }));
    expect(get(s).base.version).toBe(V2);
    expect(get(s.dirty)).toBe(false);
  });
  it('acceptCurrent replaces draft with the server version', () => {
    const s = createEditorStore(mockFile('a.md', { content: 'a' }));
    s.edit('saya');
    s.setConflict(current);
    s.acceptCurrent();
    expect(get(s).draft).toBe('baru');
    expect(get(s).base.version).toBe(V2);
  });
  it('rebaseOnCurrent keeps my draft with the new version', () => {
    const s = createEditorStore(mockFile('a.md', { content: 'a' }));
    s.edit('saya');
    s.setConflict(current);
    s.rebaseOnCurrent();
    expect(get(s).draft).toBe('saya');
    expect(get(s).base.version).toBe(V2);
    expect(get(s.dirty)).toBe(true);
  });
});

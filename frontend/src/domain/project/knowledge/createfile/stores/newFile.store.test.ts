import { get } from 'svelte/store';
import { describe, expect, it } from 'vitest';
import { createNewFileStore } from './newFile.store';

describe('createNewFileStore', () => {
  it('starts with the directory as path prefix', () => {
    expect(get(createNewFileStore('docs')).path).toBe('docs/');
    expect(get(createNewFileStore('')).path).toBe('');
  });

  it('reports a path error only after the field is touched', () => {
    const s = createNewFileStore('');
    s.setContent('x');
    expect(get(s.pathError)).toBeNull();
    s.touch();
    expect(get(s.pathError)).toBe('path_empty');
  });

  it('validates new file paths like createfile', () => {
    const s = createNewFileStore('');
    s.setPath('../x.md');
    expect(get(s.pathError)).toBe('path_traversal');
    expect(get(s.valid)).toBe(false);
    s.setPath('.env');
    expect(get(s.pathError)).toBe('hidden');
    s.setPath('config.yaml');
    expect(get(s.pathError)).toBe('unsupported_type');
    s.setPath('docs/ok.md');
    expect(get(s.pathError)).toBeNull();
    expect(get(s.valid)).toBe(true);
  });

  it('is not dirty at the start, dirty after typing a path or content', () => {
    const s = createNewFileStore('docs');
    expect(get(s.dirty)).toBe(false);
    s.setPath('docs/a.md');
    expect(get(s.dirty)).toBe(true);
    s.setPath('docs/');
    expect(get(s.dirty)).toBe(false);
    s.setContent('halo');
    expect(get(s.dirty)).toBe(true);
  });

  it('keeps an initial content as the clean baseline', () => {
    const s = createNewFileStore('', 'draf');
    expect(get(s.dirty)).toBe(false);
    s.setContent('draf!');
    expect(get(s.dirty)).toBe(true);
  });

  it('editing the path clears a server path error and the exists flag', () => {
    const s = createNewFileStore('');
    s.setPath('docs/a.md');
    s.pathFailed('sudah ada', true);
    expect(get(s).serverPathError).toBe('sudah ada');
    expect(get(s).exists).toBe(true);
    s.setPath('docs/b.md');
    expect(get(s).serverPathError).toBeNull();
    expect(get(s).exists).toBe(false);
  });

  it('formFailed replaces path errors and clearErrors resets everything', () => {
    const s = createNewFileStore('');
    s.pathFailed('x', true);
    s.formFailed('terlalu besar');
    expect(get(s)).toMatchObject({ formError: 'terlalu besar', serverPathError: null, exists: false });
    s.clearErrors();
    expect(get(s).formError).toBeNull();
  });

  it('tracks submitting', () => {
    const s = createNewFileStore('');
    s.submitting(true);
    expect(get(s).submitting).toBe(true);
  });
});

import { describe, expect, it } from 'vitest';
import { ancestors, dirName, isHidden, joinPath, normalizePrefix, validateNewFilePath, validatePath, validatePrefix } from './path';

describe('validatePath', () => {
  it.each([
    ['docs/auth/login.md', null],
    ['', 'path_empty'],
    ['/a.md', 'path_absolute'],
    ['../x.md', 'path_traversal'],
    ['a/./b.md', 'path_traversal'],
    ['a//b.md', 'path_empty_segment'],
    ['a/', 'path_empty_segment'],
    ['a\u0000b.md', 'path_control_char'],
    ['a\nb.md', 'path_control_char'],
    ['Docs/A.MD', null],
    ['a\\b.md', null],
  ])('%j -> %s', (p, r) => expect(validatePath(p)).toBe(r));
  it('length limits count runes', () => {
    const long = ('x'.repeat(100) + '/').repeat(6);
    expect(validatePath(long.slice(0, 512))).toBeNull();
    expect(validatePath(long.slice(0, 513))).toBe('path_too_long');
    expect(validatePath('y'.repeat(256))).toBe('segment_too_long');
    expect(validatePath('\u{1F600}'.repeat(255))).toBeNull();
  });
});

describe('new file path & prefix helpers', () => {
  it('validateNewFilePath', () => {
    expect(validateNewFilePath('.env')).toBe('hidden');
    expect(validateNewFilePath('config.yaml')).toBe('unsupported_type');
    expect(validateNewFilePath('docs/x.MD')).toBeNull();
  });
  it('prefix', () => {
    expect(validatePrefix('')).toBeNull();
    expect(validatePrefix('docs/')).toBeNull();
    expect(normalizePrefix('docs/')).toBe('docs');
  });
  it('ancestors, dirName, joinPath, isHidden', () => {
    expect(ancestors('docs/auth/x.md')).toEqual(['docs', 'docs/auth']);
    expect(ancestors('x.md')).toEqual([]);
    expect(dirName('docs/a.md')).toBe('docs');
    expect(dirName('a.md')).toBe('');
    expect(joinPath('', 'a.md')).toBe('a.md');
    expect(joinPath('docs', 'a.md')).toBe('docs/a.md');
    expect(isHidden('a/.git/config')).toBe(true);
  });
});

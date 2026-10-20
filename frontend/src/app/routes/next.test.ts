import { describe, expect, it } from 'vitest';
import { safeNext } from './next';

describe('safeNext', () => {
  it.each(['/', '/projects/1?tab=key', '/admin/users#x'])('accepts %s', (v) => expect(safeNext(v)).toBe(v));
  it.each([
    null, '', 'projects', '//evil.com', '/\\evil.com', 'https://evil.com', 'javascript:alert(1)',
    '/a\nb', '/a\tb', '/login', '/login?next=/x', '/change-password', '/change-password/', '/' + 'a'.repeat(2048),
  ])('rejects %j', (v) => expect(safeNext(v)).toBeNull());
});

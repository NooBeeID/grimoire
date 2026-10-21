import { describe, expect, it } from 'vitest';
import { stemRange } from './stemRange';

describe('stemRange', () => {
  it.each([
    ['docs/auth/login.md', [10, 15]],
    ['login.md', [0, 5]],
    ['docs/catatan', [5, 12]],
    ['docs/.env', [5, 9]],
    ['a.b.md', [0, 3]],
    ['', [0, 0]],
  ])('%s', (path, range) => expect(stemRange(path)).toEqual(range));
});

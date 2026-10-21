import { describe, expect, it } from 'vitest';
import { mockDocument } from '../mocks/detail-3.0';
import { mockDocument31 } from '../mocks/detail-3.1';
import { allSchemes, isNoAuth, usedSchemes } from './security';

describe('usedSchemes', () => {
  it('returns only the schemes referenced by the requirement, with scopes', () => {
    const used = usedSchemes(mockDocument31, [{ oauth: ['pets:read'] }]);
    expect(used).toHaveLength(1);
    expect(used[0]).toMatchObject({ name: 'oauth', scopes: ['pets:read'] });
    expect(used[0]!.scheme).toMatchObject({ type: 'oauth2' });
  });
  it('unknown names give scheme null; duplicates merge scopes', () => {
    const used = usedSchemes(mockDocument, [{ bearer: [], ghost: ['a'] }, { ghost: ['b', 'a'] }]);
    expect(used.map((u) => u.name)).toEqual(['bearer', 'ghost']);
    expect(used[1]).toMatchObject({ scheme: null, scopes: ['a', 'b'] });
  });
  it('resolves $ref schemes and tolerates bad input', () => {
    const doc = { components: { securitySchemes: { a: { $ref: '#/components/securitySchemes/b' }, b: { type: 'http', scheme: 'basic' } } } };
    expect(usedSchemes(doc, [{ a: [] }])[0]!.scheme).toMatchObject({ scheme: 'basic' });
    expect(usedSchemes({}, null)).toEqual([]);
    expect(usedSchemes({}, [5, null, { x: 'not-array' }])).toEqual([{ name: 'x', scheme: null, scopes: [] }]);
  });
});

describe('isNoAuth', () => {
  it('[] and {} mean no auth; null (undefined) does not', () => {
    expect(isNoAuth([])).toBe(true);
    expect(isNoAuth([{ k: [] }, {}])).toBe(true);
    expect(isNoAuth([{ k: [] }])).toBe(false);
    expect(isNoAuth(null)).toBe(false);
  });
});

describe('allSchemes', () => {
  it('lists every components.securitySchemes entry', () => {
    expect(allSchemes(mockDocument).map((s) => [s.name, s.type])).toEqual([['bearer', 'http'], ['apiKey', 'apiKey']]);
    expect(allSchemes({})).toEqual([]);
  });
});

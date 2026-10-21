import { describe, expect, it, vi } from 'vitest';
import { deref, refName, resolvePointer } from './refs';

const doc = {
  a: { 'x/y': { '~z': [10, 20] } },
  components: {
    schemas: {
      A: { $ref: '#/components/schemas/B' },
      B: { $ref: '#/components/schemas/C' },
      C: { type: 'string' },
      L1: { $ref: '#/components/schemas/L2' },
      L2: { $ref: '#/components/schemas/L1' },
    },
  },
};

describe('refs', () => {
  it('resolves pointers with escapes, array indexes and root', () => {
    expect(resolvePointer(doc, '#/a/x~1y/~0z/1')).toBe(20);
    expect(resolvePointer(doc, '#')).toBe(doc);
    expect(resolvePointer(doc, '#/nope')).toBeUndefined();
    expect(resolvePointer(doc, '#/a/x~1y/~0z/9')).toBeUndefined();
  });
  it('follows a chain A -> B -> C', () => {
    expect(deref(doc, { $ref: '#/components/schemas/A' })).toMatchObject({ node: { type: 'string' }, ref: '#/components/schemas/A', circular: false, unresolved: false });
  });
  it('flags circular chains and refs already on the stack', () => {
    expect(deref(doc, { $ref: '#/components/schemas/L1' }).circular).toBe(true);
    expect(deref(doc, { $ref: '#/components/schemas/C' }, ['#/components/schemas/C']).circular).toBe(true);
  });
  it('flags unresolved refs and never fetches external ones', () => {
    const fetchSpy = vi.fn();
    vi.stubGlobal('fetch', fetchSpy);
    expect(deref(doc, { $ref: '#/missing' }).unresolved).toBe(true);
    expect(deref(doc, { $ref: 'http://x/y.yaml#/X' }).unresolved).toBe(true);
    expect(fetchSpy).not.toHaveBeenCalled();
    vi.unstubAllGlobals();
  });
  it('merges sibling description (3.1) over the target', () => {
    expect(deref(doc, { $ref: '#/components/schemas/C', description: 'x' }).node).toMatchObject({ type: 'string', description: 'x' });
  });
  it('refName extracts and unescapes the last segment', () => {
    expect(refName('#/components/schemas/Customer')).toBe('Customer');
    expect(refName('#/components/schemas/a~1b')).toBe('a/b');
  });
});

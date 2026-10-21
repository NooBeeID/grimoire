import { describe, expect, it } from 'vitest';
import { examplesOf, toPrettyJson, TRUNCATED_MARK } from './examples';

describe('examplesOf', () => {
  const doc = { components: { examples: { One: { summary: 'Satu', value: { a: 1 } } } } };
  it('reads a single example', () => {
    expect(examplesOf(doc, { example: { x: 1 } })).toEqual([{ name: 'example', summary: null, value: { x: 1 }, externalValue: null }]);
    expect(examplesOf(doc, { example: null })[0]!.value).toBeNull();
  });
  it('reads the examples map including $ref entries', () => {
    const out = examplesOf(doc, { examples: { satu: { $ref: '#/components/examples/One' }, dua: { summary: 'Dua', value: 2 } } });
    expect(out).toEqual([
      { name: 'satu', summary: 'Satu', value: { a: 1 }, externalValue: null },
      { name: 'dua', summary: 'Dua', value: 2, externalValue: null },
    ]);
  });
  it('externalValue is kept as text and never fetched', () => {
    const out = examplesOf(doc, { examples: { e: { externalValue: 'https://x.example/e.json' } } });
    expect(out[0]).toMatchObject({ value: undefined, externalValue: 'https://x.example/e.json' });
  });
  it('skips unresolved refs and tolerates odd input', () => {
    expect(examplesOf(doc, { examples: { bad: { $ref: '#/nope' } } })).toEqual([]);
    expect(examplesOf(doc, 5)).toEqual([]);
    expect(examplesOf(doc, { examples: 5 })).toEqual([]);
  });
});

describe('toPrettyJson', () => {
  it('pretty prints objects and quotes strings', () => {
    expect(toPrettyJson({ a: 1 })).toBe('{\n  "a": 1\n}');
    expect(toPrettyJson('x')).toBe('"x"');
    expect(toPrettyJson(undefined)).toBe('undefined');
  });
  it('truncates long output with a marker', () => {
    const out = toPrettyJson({ s: 'x'.repeat(100) }, 20);
    expect(out.startsWith('{\n  "s": "xxx')).toBe(true);
    expect(out.endsWith(TRUNCATED_MARK)).toBe(true);
    expect(out.length).toBe(20 + TRUNCATED_MARK.length);
  });
  it('does not throw on cyclic values', () => {
    const o: Record<string, unknown> = {};
    o.self = o;
    expect(() => toPrettyJson(o)).not.toThrow();
  });
});

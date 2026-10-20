import { describe, expect, it } from 'vitest';
import id from './id.json';
import en from './en.json';

describe('i18n key parity', () => {
  it('id and en have identical key sets', () => {
    expect(Object.keys(en).sort()).toEqual(Object.keys(id).sort());
  });
});

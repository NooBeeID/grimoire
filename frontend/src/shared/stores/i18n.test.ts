import { get } from 'svelte/store';
import { afterEach, describe, expect, it, vi } from 'vitest';
import id from '../i18n/id.json';
import en from '../i18n/en.json';
import { locale, setLocale, t } from './i18n';

afterEach(() => {
  setLocale('id');
  localStorage.clear();
});

describe('i18n', () => {
  it('translates and switches locale, persisting and setting lang', () => {
    expect(get(t)('home.title')).toBe(id['home.title']);
    setLocale('en');
    expect(get(t)('home.title')).toBe(en['home.title']);
    expect(localStorage.getItem('grimoire.locale')).toBe('en');
    expect(document.documentElement.lang).toBe('en');
    expect(get(locale)).toBe('en');
  });
  it('interpolates', () => {
    expect(get(t)('pagination.page_of', { page: 2, pages: 5 })).toBe('Halaman 2 dari 5');
  });
  it('missing key returns key and warns in DEV', () => {
    const w = vi.spyOn(console, 'warn').mockImplementation(() => {});
    expect(get(t)('no.such.key')).toBe('no.such.key');
    expect(w).toHaveBeenCalled();
  });
  it('falls back to id then key', () => {
    setLocale('en');
    vi.spyOn(console, 'warn').mockImplementation(() => {});
    expect(get(t)('nothing')).toBe('nothing');
  });
  it('setLocale survives throwing localStorage', () => {
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('blocked');
    });
    expect(() => setLocale('en')).not.toThrow();
    expect(get(locale)).toBe('en');
  });
  it('dictionaries have identical, non-empty keys', () => {
    expect(Object.keys(id).sort()).toEqual(Object.keys(en).sort());
    for (const v of [...Object.values(id), ...Object.values(en)]) expect(v.trim()).not.toBe('');
  });
});

describe('initial locale', () => {
  async function fresh() {
    vi.resetModules();
    return (await import('./i18n')).locale;
  }
  it('defaults to id', async () => {
    vi.spyOn(navigator, 'language', 'get').mockReturnValue('id-ID');
    expect(get(await fresh())).toBe('id');
  });
  it('uses navigator en', async () => {
    vi.spyOn(navigator, 'language', 'get').mockReturnValue('en-US');
    expect(get(await fresh())).toBe('en');
  });
  it('localStorage wins; invalid ignored', async () => {
    vi.spyOn(navigator, 'language', 'get').mockReturnValue('id-ID');
    localStorage.setItem('grimoire.locale', 'en');
    expect(get(await fresh())).toBe('en');
    localStorage.setItem('grimoire.locale', 'fr');
    expect(get(await fresh())).toBe('id');
  });
  it('throwing localStorage -> id', async () => {
    vi.spyOn(navigator, 'language', 'get').mockReturnValue('id-ID');
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('blocked');
    });
    expect(get(await fresh())).toBe('id');
  });
});

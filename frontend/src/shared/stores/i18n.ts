import { derived, writable, type Readable } from 'svelte/store';
import id from '../i18n/id.json';
import en from '../i18n/en.json';

export type Locale = 'id' | 'en';
export const SUPPORTED_LOCALES: readonly Locale[] = ['id', 'en'];

const STORAGE_KEY = 'grimoire.locale';
const dicts: Record<Locale, Record<string, string>> = { id, en };

function isLocale(v: unknown): v is Locale {
  return v === 'id' || v === 'en';
}

function initialLocale(): Locale {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (isLocale(stored)) return stored;
  } catch {
    // storage tidak tersedia
  }
  if (typeof navigator !== 'undefined' && navigator.language?.toLowerCase().startsWith('en')) return 'en';
  return 'id';
}

export const locale = writable<Locale>(initialLocale());

export type Translate = (key: string, params?: Record<string, string | number>) => string;

export const t: Readable<Translate> = derived(
  locale,
  ($locale): Translate => (key, params) => {
    const raw = dicts[$locale][key] ?? dicts.id[key];
    if (raw === undefined) {
      if (import.meta.env.DEV) console.warn(`[i18n] missing key: ${key}`);
      return key;
    }
    if (!params) return raw;
    return raw.replace(/\{(\w+)\}/g, (m, name: string) => (name in params ? String(params[name]) : m));
  },
);

export function setLocale(l: Locale): void {
  locale.set(l);
  try {
    localStorage.setItem(STORAGE_KEY, l);
  } catch {
    // diabaikan
  }
  if (typeof document !== 'undefined') document.documentElement.lang = l;
}

import { writable } from 'svelte/store';

export type Theme = 'light' | 'dark';

const STORAGE_KEY = 'grimoire.theme';

function stored(): Theme | null {
  try {
    const v = localStorage.getItem(STORAGE_KEY);
    return v === 'light' || v === 'dark' ? v : null;
  } catch {
    return null;
  }
}

const query = typeof window !== 'undefined' && window.matchMedia ? window.matchMedia('(prefers-color-scheme: dark)') : null;
const system = (): Theme => (query?.matches ? 'dark' : 'light');

// Logika awal sama dengan public/theme-init.js (yang berjalan sebelum render); ubah keduanya bersama.
export const theme = writable<Theme>(stored() ?? system());

theme.subscribe((t) => {
  if (typeof document !== 'undefined') document.documentElement.dataset.theme = t;
});

// Selama pengguna belum memilih sendiri, ikuti perubahan tema sistem.
query?.addEventListener?.('change', () => {
  if (stored() === null) theme.set(system());
});

export function toggleTheme(): void {
  theme.update((cur) => {
    const next: Theme = cur === 'dark' ? 'light' : 'dark';
    try {
      localStorage.setItem(STORAGE_KEY, next);
    } catch {
      // storage tidak tersedia: pilihan berlaku untuk sesi ini saja
    }
    return next;
  });
}

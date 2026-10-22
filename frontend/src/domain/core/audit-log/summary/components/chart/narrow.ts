import { readable } from 'svelte/store';

/** Ponsel: lebar di bawah 640px (breakpoint `sm`). Jatuh ke false bila matchMedia tidak ada. */
const QUERY = '(max-width: 639px)';

export const narrow = readable(false, (set) => {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return;
  const mq = window.matchMedia(QUERY);
  set(mq.matches);
  const on = (e: MediaQueryListEvent) => set(e.matches);
  mq.addEventListener('change', on);
  return () => mq.removeEventListener('change', on);
});

export const MOBILE_PLOT_HEIGHT = 180;
export const MOBILE_MAX_TICKS = 4;
export const DESKTOP_MAX_TICKS = 8;

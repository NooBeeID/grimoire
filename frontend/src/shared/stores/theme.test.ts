import { get } from 'svelte/store';
import { beforeEach, describe, expect, it } from 'vitest';
import { theme, toggleTheme } from './theme';

describe('theme store', () => {
  beforeEach(() => {
    localStorage.clear();
    theme.set('light');
  });

  it('toggles between light and dark, sets data-theme, and remembers the choice', () => {
    toggleTheme();
    expect(get(theme)).toBe('dark');
    expect(document.documentElement.dataset.theme).toBe('dark');
    expect(localStorage.getItem('grimoire.theme')).toBe('dark');
    toggleTheme();
    expect(get(theme)).toBe('light');
    expect(document.documentElement.dataset.theme).toBe('light');
    expect(localStorage.getItem('grimoire.theme')).toBe('light');
  });
});

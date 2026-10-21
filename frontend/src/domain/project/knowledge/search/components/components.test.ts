import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { setLocale } from '../../../../../shared/stores/i18n';
import ScopeToggle from './ScopeToggle.svelte';
import SearchResults from './SearchResults.svelte';

beforeEach(() => setLocale('id'));
const item = (snippet: { text: string; highlight: boolean }[]) => ({ path: 'docs/auth/login.md', title: 'Login', score: 1, snippet });

describe('SearchResults', () => {
  it('highlights as a single <mark> and total text', () => {
    render(SearchResults, { items: [item([{ text: 'Alur ', highlight: false }, { text: 'login', highlight: true }])], total: 1, limit: 20, onopen: vi.fn(), onmore: vi.fn() });
    expect(document.querySelectorAll('mark')).toHaveLength(1);
    expect(document.querySelector('mark')!.textContent).toBe('login');
    expect(screen.getByText('1 dari 1 hasil')).toBeInTheDocument();
  });
  it('T05-AC5: snippet markup is shown literally', () => {
    render(SearchResults, { items: [item([{ text: '<script>alert(1)</script>', highlight: false }])], total: 1, limit: 20, onopen: vi.fn(), onmore: vi.fn() });
    expect(document.querySelector('script')).toBeNull();
    expect(screen.getByText('<script>alert(1)</script>')).toBeInTheDocument();
  });
  it('shows "more" only when total exceeds items and limit < 50', async () => {
    const onmore = vi.fn();
    const { rerender } = render(SearchResults, { items: [item([])], total: 5, limit: 20, onopen: vi.fn(), onmore });
    await userEvent.click(screen.getByRole('button', { name: 'Tampilkan lebih banyak' }));
    expect(onmore).toHaveBeenCalled();
    await rerender({ items: [item([])], total: 5, limit: 50, onopen: vi.fn(), onmore });
    expect(screen.queryByRole('button', { name: 'Tampilkan lebih banyak' })).toBeNull();
  });
  it('opens a result', async () => {
    const onopen = vi.fn();
    render(SearchResults, { items: [item([])], total: 1, limit: 20, onopen, onmore: vi.fn() });
    await userEvent.click(screen.getByRole('button', { name: /Login/ }));
    expect(onopen).toHaveBeenCalledWith('docs/auth/login.md');
  });
});

describe('ScopeToggle', () => {
  it('renders nothing without a context folder', () => {
    render(ScopeToggle, { contextDir: '', scope: '', onchange: vi.fn() });
    expect(screen.queryByRole('checkbox')).not.toBeInTheDocument();
  });
  it('is unchecked at the root scope and scopes to the folder when checked', async () => {
    const onchange = vi.fn();
    render(ScopeToggle, { contextDir: 'docs/auth', scope: '', onchange });
    const box = screen.getByRole('checkbox', { name: 'Hanya di docs/auth' });
    expect(box).not.toBeChecked();
    await userEvent.click(box);
    expect(onchange).toHaveBeenCalledWith('docs/auth');
  });
  it('is checked when scoped and clears the scope when unchecked', async () => {
    const onchange = vi.fn();
    render(ScopeToggle, { contextDir: 'docs/auth', scope: 'docs/auth', onchange });
    const box = screen.getByRole('checkbox');
    expect(box).toBeChecked();
    await userEvent.click(box);
    expect(onchange).toHaveBeenCalledWith('');
  });
  it('renders a folder name with HTML literally', () => {
    render(ScopeToggle, { contextDir: '<b>x</b>', scope: '', onchange: vi.fn() });
    expect(document.querySelector('b')).toBeNull();
  });
});

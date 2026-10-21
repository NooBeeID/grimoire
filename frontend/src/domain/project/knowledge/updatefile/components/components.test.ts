import { fireEvent, render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { setLocale } from '../../../../../shared/stores/i18n';
import { V2 } from '../../base';
import CompareView from './CompareView.svelte';
import ConflictDialog from './ConflictDialog.svelte';
import EditorSplit from './EditorSplit.svelte';
import EditorToolbar from './EditorToolbar.svelte';

beforeEach(() => setLocale('id'));
afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe('EditorToolbar', () => {
  const props = (over: Record<string, unknown> = {}) => ({
    path: 'docs/auth/login.md',
    dirty: false,
    saving: false,
    pane: 'write' as const,
    onpane: vi.fn(),
    onsave: vi.fn(),
    oncancel: vi.fn(),
    ...over,
  });

  it('shows the path in mono and the saved state', () => {
    render(EditorToolbar, props());
    expect(screen.getByText('docs/auth/login.md')).toHaveClass('font-mono');
    expect(screen.getByText('Tersimpan')).toBeInTheDocument();
  });
  it('announces unsaved changes politely', () => {
    render(EditorToolbar, props({ dirty: true }));
    const status = screen.getByText('Belum disimpan');
    expect(status).toHaveAttribute('aria-live', 'polite');
  });
  it('calls save and cancel', async () => {
    const p = props();
    render(EditorToolbar, p);
    await userEvent.click(screen.getByRole('button', { name: 'Simpan' }));
    await userEvent.click(screen.getByRole('button', { name: 'Batal' }));
    expect(p.onsave).toHaveBeenCalledTimes(1);
    expect(p.oncancel).toHaveBeenCalledTimes(1);
  });
  it('toggles between write and preview', async () => {
    const p = props();
    render(EditorToolbar, p);
    expect(screen.getByRole('button', { name: 'Tulis' })).toHaveAttribute('aria-pressed', 'true');
    await userEvent.click(screen.getByRole('button', { name: 'Pratinjau' }));
    expect(p.onpane).toHaveBeenCalledWith('preview');
  });
});

describe('EditorSplit', () => {
  it('renders the preview after a debounce, not on every keystroke', async () => {
    vi.useFakeTimers();
    render(EditorSplit, { value: '# Awal', path: 'a.md', label: 'Isi Markdown' });
    await vi.advanceTimersByTimeAsync(300);
    expect(screen.getByRole('heading', { level: 1, name: 'Awal' })).toBeInTheDocument();
    await fireEvent.input(screen.getByLabelText('Isi Markdown'), { target: { value: '# Baru' } });
    await vi.advanceTimersByTimeAsync(100);
    expect(screen.queryByRole('heading', { name: 'Baru' })).not.toBeInTheDocument();
    await vi.advanceTimersByTimeAsync(200);
    expect(screen.getByRole('heading', { level: 1, name: 'Baru' })).toBeInTheDocument();
  });
  it('shows .txt content as preformatted text', async () => {
    render(EditorSplit, { value: '# bukan heading', path: 'catatan.txt', label: 'Isi Markdown' });
    expect(screen.queryByRole('heading')).not.toBeInTheDocument();
    expect(document.querySelector('pre')).toHaveTextContent('# bukan heading');
  });
  it('switches to a manual preview above 200 KB', async () => {
    vi.useFakeTimers();
    const big = '# Besar\n' + 'x'.repeat(200 * 1024 + 10);
    render(EditorSplit, { value: big, path: 'a.md', label: 'Isi Markdown' });
    await vi.advanceTimersByTimeAsync(400);
    const refresh = screen.getByRole('button', { name: 'Perbarui pratinjau' });
    await fireEvent.input(screen.getByLabelText('Isi Markdown'), { target: { value: big.replace('# Besar', '# Diubah') } });
    await vi.advanceTimersByTimeAsync(400);
    expect(screen.queryByRole('heading', { name: 'Diubah' })).not.toBeInTheDocument();
    await fireEvent.click(refresh);
    expect(screen.getByRole('heading', { level: 1, name: 'Diubah' })).toBeInTheDocument();
  });
  it('hides the editor on narrow screens when the preview pane is chosen', () => {
    const { container } = render(EditorSplit, { value: 'x', path: 'a.md', label: 'Isi Markdown', pane: 'preview' });
    expect(container.querySelector('.max-lg\\:hidden')).toContainElement(screen.getByLabelText('Isi Markdown'));
  });
});

describe('ConflictDialog', () => {
  const conflict = { content: 'versi sari', version: V2, updated_at: '2026-10-21T09:30:00Z', updated_by_label: 'Sari' };
  const props = (over: Record<string, unknown> = {}) => ({
    conflict,
    mine: 'versi saya',
    compareOpen: false,
    saving: false,
    oncompare: vi.fn(),
    onreload: vi.fn(),
    onkeep: vi.fn(),
    onoverwrite: vi.fn(),
    ...over,
  });

  it('names who saved and when, without showing a compare view yet', () => {
    render(ConflictDialog, props());
    expect(screen.getByText(/Sari menyimpan perubahan pada/)).toBeInTheDocument();
    expect(screen.queryByTestId('compare-mine')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Timpa dengan versi saya' })).not.toBeInTheDocument();
  });
  it('offers compare, reload and keep editing', async () => {
    const p = props();
    render(ConflictDialog, p);
    await userEvent.click(screen.getByRole('button', { name: 'Bandingkan' }));
    await userEvent.click(screen.getByRole('button', { name: 'Muat versi terbaru' }));
    await userEvent.click(screen.getByRole('button', { name: 'Lanjut mengedit' }));
    expect(p.oncompare).toHaveBeenCalled();
    expect(p.onreload).toHaveBeenCalled();
    expect(p.onkeep).toHaveBeenCalled();
  });
  it('offers an explicit overwrite only from the compare view', async () => {
    const p = props({ compareOpen: true });
    render(ConflictDialog, p);
    expect(screen.getByTestId('compare-theirs')).toHaveTextContent('versi sari');
    await userEvent.click(screen.getByRole('button', { name: 'Timpa dengan versi saya' }));
    expect(p.onoverwrite).toHaveBeenCalledTimes(1);
    await userEvent.click(screen.getByRole('button', { name: 'Pakai versi terbaru' }));
    expect(p.onreload).toHaveBeenCalledTimes(1);
  });
  it('renders nothing open without a conflict', () => {
    render(ConflictDialog, props({ conflict: null }));
    expect(screen.queryByText(/menyimpan perubahan pada/)).not.toBeInTheDocument();
  });
});

describe('CompareView', () => {
  it('shows both versions and marks added and removed lines', () => {
    render(CompareView, { mine: 'a\nsaya\nc', theirs: 'a\nsari\nc' });
    expect(screen.getByTestId('compare-mine')).toHaveTextContent('saya');
    expect(screen.getByTestId('compare-theirs')).toHaveTextContent('sari');
    const rows = screen.getAllByRole('listitem');
    expect(rows.map((r) => r.getAttribute('data-op'))).toEqual(['eq', 'del', 'add', 'eq']);
  });
  it('renders HTML in either version literally', () => {
    render(CompareView, { mine: '<img src=x onerror=alert(1)>', theirs: '<script>alert(1)</script>' });
    expect(document.querySelector('img, script')).toBeNull();
    expect(screen.getByTestId('compare-mine')).toHaveTextContent('<img src=x onerror=alert(1)>');
  });
  it('omits the highlight list for a very large diff but keeps both columns', () => {
    const big = Array.from({ length: 5001 }, (_, i) => `l${i}`).join('\n');
    render(CompareView, { mine: big, theirs: 'kecil' });
    expect(screen.queryByRole('list')).not.toBeInTheDocument();
    expect(screen.getByTestId('compare-theirs')).toHaveTextContent('kecil');
  });
  it('copies my version to the clipboard', async () => {
    const writeText = vi.fn(async () => {});
    vi.stubGlobal('navigator', { ...navigator, clipboard: { writeText } });
    render(CompareView, { mine: 'draf saya', theirs: 'lain' });
    await userEvent.click(screen.getByRole('button', { name: 'Salin versi saya' }));
    expect(writeText).toHaveBeenCalledWith('draf saya');
  });
});

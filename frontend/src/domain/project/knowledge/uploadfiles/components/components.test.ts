import { fireEvent, render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { setLocale } from '../../../../../shared/stores/i18n';
import type { Filter } from '../stores/upload.store';
import type { LocalFile, ReportItem, UploadPlan, UploadTarget } from '../types';
import DropZone from './DropZone.svelte';
import UploadProgress from './UploadProgress.svelte';
import UploadReport from './UploadReport.svelte';
import UploadReview from './UploadReview.svelte';

beforeEach(() => setLocale('id'));

const lf = (path: string, size = 10): LocalFile => ({ path, file: new File(['x'.repeat(size)], path.split('/').pop()!) });
const target = (key: string, labelKey: string): UploadTarget => ({ key, labelKey, accepts: () => true, upload: async () => [] });
const knowledge = target('knowledge', 'knowledge.upload.target.knowledge');
const rpt = (path: string, status: ReportItem['status'], reasonText?: string): ReportItem => ({ path, status, reasonText, target: 'knowledge' });
const counts = (over: Partial<Record<'created' | 'updated' | 'unchanged' | 'skipped' | 'rejected' | 'unsupported', number>> = {}) => ({
  created: 0, updated: 0, unchanged: 0, skipped: 0, rejected: 0, unsupported: 0, ...over,
});

describe('DropZone', () => {
  it('offers a folder picker and a file picker limited to supported extensions', () => {
    render(DropZone, { onpick: vi.fn() });
    expect(screen.getByTestId('pick-folder')).toHaveAttribute('webkitdirectory');
    expect(screen.getByTestId('pick-files')).toHaveAttribute('accept', '.md,.markdown,.txt');
  });
  it('adds the extensions announced by extra targets', () => {
    render(DropZone, { onpick: vi.fn(), acceptExts: ['.yaml', '.json'] });
    expect(screen.getByTestId('pick-files')).toHaveAttribute('accept', '.md,.markdown,.txt,.yaml,.json');
  });
  it('reports files chosen from the file picker', async () => {
    const onpick = vi.fn();
    render(DropZone, { onpick });
    await userEvent.upload(screen.getByTestId('pick-files'), [new File(['a'], 'a.md'), new File(['b'], 'b.txt')]);
    expect(onpick).toHaveBeenCalledTimes(1);
    const [files, truncated] = onpick.mock.calls[0]!;
    expect(files.map((f: LocalFile) => f.path)).toEqual(['a.md', 'b.txt']);
    expect(truncated).toBe(false);
  });
  it('reports dropped files and highlights while dragging over', async () => {
    const onpick = vi.fn();
    render(DropZone, { onpick });
    const zone = screen.getByTestId('dropzone');
    expect(zone).toHaveAttribute('data-over', 'false');
    await fireEvent.dragEnter(zone);
    expect(zone).toHaveAttribute('data-over', 'true');
    await fireEvent.dragLeave(zone);
    expect(zone).toHaveAttribute('data-over', 'false');
    const files = [new File(['a'], 'a.md'), new File(['b'], 'b.md')];
    await fireEvent.drop(zone, { dataTransfer: { items: [], files } });
    await vi.waitFor(() => expect(onpick).toHaveBeenCalled());
    expect(onpick.mock.calls[0]![0].map((f: LocalFile) => f.path)).toEqual(['a.md', 'b.md']);
    expect(zone).toHaveAttribute('data-over', 'false');
  });
  it('ignores a drop without files', async () => {
    const onpick = vi.fn();
    render(DropZone, { onpick });
    await fireEvent.drop(screen.getByTestId('dropzone'), { dataTransfer: { items: [], files: [] } });
    await Promise.resolve();
    expect(onpick).not.toHaveBeenCalled();
  });
});

describe('UploadReview', () => {
  const plan = (over: Partial<UploadPlan> = {}): UploadPlan => ({
    byTarget: new Map([['knowledge', [lf('docs/a.md', 1024), lf('docs/b.md', 1024)]]]),
    skipped: [],
    unsupported: [],
    totalBytes: 2048,
    ...over,
  });
  const props = (over: Record<string, unknown> = {}) => ({
    plan: plan(),
    targets: [knowledge],
    targetDir: '',
    onstart: vi.fn(),
    oncancel: vi.fn(),
    ...over,
  });

  it('shows the count and size per target and the destination', () => {
    render(UploadReview, props({ targetDir: 'docs' }));
    expect(screen.getByText('2 file ke Knowledge (2 KB)')).toBeInTheDocument();
    expect(screen.getByText('Tujuan: docs')).toBeInTheDocument();
  });
  it('names the project root when there is no folder', () => {
    render(UploadReview, props());
    expect(screen.getByText('Tujuan: root project')).toBeInTheDocument();
  });
  it('lists one line per target that has files', () => {
    const second = target('spec', 'knowledge.search.label');
    const p = plan({ byTarget: new Map([['knowledge', [lf('a.md')]], ['spec', [lf('api.yaml', 2048)]]]) });
    render(UploadReview, props({ plan: p, targets: [knowledge, second] }));
    expect(screen.getByText(/1 file ke Knowledge/)).toBeInTheDocument();
    expect(screen.getByText('1 file ke Cari knowledge (2 KB)')).toBeInTheDocument();
  });
  it('disables upload when nothing can be sent', () => {
    render(UploadReview, props({ plan: plan({ byTarget: new Map(), unsupported: [rpt('logo.png', 'unsupported')] }) }));
    expect(screen.getByText('Tidak ada file yang bisa diunggah')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Unggah' })).toBeDisabled();
  });
  it('starts and cancels', async () => {
    const p = props();
    render(UploadReview, p);
    await userEvent.click(screen.getByRole('button', { name: 'Unggah' }));
    await userEvent.click(screen.getByRole('button', { name: 'Batal' }));
    expect(p.onstart).toHaveBeenCalledTimes(1);
    expect(p.oncancel).toHaveBeenCalledTimes(1);
  });
  it('summarises skipped and unsupported files and lists their paths as text', () => {
    render(UploadReview, props({ plan: plan({ skipped: [rpt('.git/config', 'skipped')], unsupported: [rpt('<img src=x>.png', 'unsupported')] }) }));
    expect(screen.getByText('1 file dilewati (tersembunyi)')).toBeInTheDocument();
    expect(screen.getByText('1 file tidak didukung')).toBeInTheDocument();
    expect(screen.getByText('.git/config')).toHaveClass('break-all');
    expect(document.querySelector('img')).toBeNull();
    expect(screen.getByText('<img src=x>.png')).toBeInTheDocument();
  });
  it('collapses long lists of skipped files', () => {
    const many = Array.from({ length: 11 }, (_, i) => rpt(`.git/o${i}`, 'skipped'));
    render(UploadReview, props({ plan: plan({ skipped: many }) }));
    expect(screen.getByText('11 file dilewati (tersembunyi)').closest('details')).not.toHaveAttribute('open');
  });
  it('warns when the dropped folder was cut at the entry limit', () => {
    render(UploadReview, props({ truncated: true }));
    expect(screen.getByRole('status')).toHaveTextContent('10000');
  });
});

describe('UploadProgress', () => {
  it('shows the percentage while sending', () => {
    render(UploadProgress, { progress: { loaded: 25, total: 100, phase: 'sending' }, targetLabel: 'Knowledge', oncancel: vi.fn() });
    expect(screen.getByText('Mengirim 25%')).toBeInTheDocument();
    expect(screen.getByText('Knowledge')).toBeInTheDocument();
    expect(document.querySelector('progress')).toHaveAttribute('value', '25');
  });
  it('shows an indeterminate state while the server processes', () => {
    render(UploadProgress, { progress: { loaded: 100, total: 100, phase: 'processing' }, oncancel: vi.fn() });
    expect(screen.getByText('Memproses di server...')).toBeInTheDocument();
    expect(document.querySelector('progress')).not.toHaveAttribute('value');
  });
  it('is indeterminate before any progress event and cancels on request', async () => {
    const oncancel = vi.fn();
    render(UploadProgress, { progress: null, oncancel });
    expect(screen.getByText('Memproses di server...')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Batalkan' }));
    expect(oncancel).toHaveBeenCalledTimes(1);
  });
  it('caps the percentage at 100', () => {
    render(UploadProgress, { progress: { loaded: 300, total: 100, phase: 'sending' }, oncancel: vi.fn() });
    expect(screen.getByText('Mengirim 100%')).toBeInTheDocument();
  });
});

describe('UploadReport', () => {
  const items = [rpt('docs/a.md', 'created'), rpt('docs/b.md', 'updated'), rpt('docs/big.md', 'rejected', 'Ukuran file melebihi batas 1 MB')];
  const props = (over: Record<string, unknown> = {}) => ({
    items,
    counts: counts({ created: 1, updated: 1, rejected: 1 }),
    filter: 'all' as Filter,
    onfilter: vi.fn(),
    ...over,
  });

  it('shows a count for each status', () => {
    render(UploadReport, props());
    for (const label of ['Dibuat', 'Ditimpa', 'Tidak berubah', 'Dilewati', 'Ditolak', 'Tidak didukung']) {
      expect(screen.getAllByText(label).length).toBeGreaterThan(0);
    }
    expect(screen.getByText('Ditolak', { selector: 'dt' }).nextElementSibling).toHaveTextContent('1');
  });
  it('renders filters as a radiogroup and reports the choice', async () => {
    const p = props({ filter: 'failed' });
    render(UploadReport, p);
    const group = screen.getByRole('radiogroup');
    expect(group).toBeInTheDocument();
    expect(screen.getByRole('radio', { name: 'Gagal' })).toHaveAttribute('aria-checked', 'true');
    expect(screen.getByRole('radio', { name: 'Semua' })).toHaveAttribute('aria-checked', 'false');
    await userEvent.click(screen.getByRole('radio', { name: 'Dibuat' }));
    expect(p.onfilter).toHaveBeenCalledWith('created');
  });
  it('lists paths and reasons as text', () => {
    render(UploadReport, props());
    expect(screen.getByText('docs/big.md')).toHaveClass('font-mono');
    expect(screen.getByText('Ukuran file melebihi batas 1 MB')).toBeInTheDocument();
  });
  it('renders a path containing HTML literally', () => {
    render(UploadReport, props({ items: [rpt('<script>alert(1)</script>.md', 'created')] }));
    expect(document.querySelector('script')).toBeNull();
    expect(screen.getByText('<script>alert(1)</script>.md')).toBeInTheDocument();
  });
  it('shows per-target error banners and the cancelled note', () => {
    render(UploadReport, props({ errors: ['Upload ke Knowledge gagal: Ukuran total upload melebihi batas 2 MB'], cancelled: true }));
    expect(screen.getByRole('alert')).toHaveTextContent('melebihi batas 2 MB');
    expect(screen.getByText('Upload dibatalkan')).toBeInTheDocument();
  });
  it('renders very long lists in steps of 200 with a show-more button', async () => {
    const many = Array.from({ length: 501 }, (_, i) => rpt(`docs/f${i}.md`, 'created'));
    render(UploadReport, props({ items: many, counts: counts({ created: 501 }) }));
    expect(screen.getAllByRole('row')).toHaveLength(1 + 200);
    await userEvent.click(screen.getByRole('button', { name: 'Tampilkan lebih banyak' }));
    expect(screen.getAllByRole('row')).toHaveLength(1 + 400);
  });
  it('renders up to 500 rows at once without a show-more button', () => {
    const exact = Array.from({ length: 500 }, (_, i) => rpt(`docs/f${i}.md`, 'created'));
    render(UploadReport, props({ items: exact }));
    expect(screen.getAllByRole('row')).toHaveLength(1 + 500);
    expect(screen.queryByRole('button', { name: 'Tampilkan lebih banyak' })).not.toBeInTheDocument();
  });
});

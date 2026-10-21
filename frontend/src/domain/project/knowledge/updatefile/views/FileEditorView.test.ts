import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { setLocale } from '../../../../../shared/stores/i18n';
import { mockFile, V1, V2 } from '../../base';
import View from './FileEditorView.svelte';

beforeEach(() => {
  setLocale('id');
  sessionStorage.clear();
});
afterEach(() => vi.unstubAllGlobals());

const json = (s: number, b: unknown) => new Response(JSON.stringify(b), { status: s });
const props = (over: Record<string, unknown> = {}) => ({
  projectId: 'p1', path: 'docs/a.md', onclose: vi.fn(), ondirtychange: vi.fn(), onnotfound: vi.fn(), onsaveasnew: vi.fn(), ...over,
});
const current = { content: 'versi budi', version: V2, updated_at: '2026-10-21T09:30:00Z', updated_by_label: 'budi@tim.id' };
const conflict = () => json(409, { error: { code: 'CONFLICT', message: 'bentrok', details: { reason: 'version_conflict', current } } });

// Urutan fetch: GET file, lalu panggilan berikutnya sesuai skenario.
function scenario(...next: Response[]) {
  const f = vi.fn();
  f.mockResolvedValueOnce(json(200, mockFile('docs/a.md', { content: 'awal' })));
  next.forEach((r) => f.mockResolvedValueOnce(r));
  vi.stubGlobal('fetch', f);
  return f;
}
const body = (f: ReturnType<typeof vi.fn>, n: number) => JSON.parse((f.mock.calls[n]![1] as RequestInit).body as string);

describe('FileEditorView', () => {
  it('T05-AC3: 409 shows the conflict dialog without overwriting; reload then save uses the new version', async () => {
    const f = scenario(conflict(), json(200, mockFile('docs/a.md', { content: 'versi budi!', version: 'c'.repeat(64) })));
    render(View, props());
    const area = await screen.findByLabelText('Isi Markdown');
    await userEvent.type(area, ' saya');
    await userEvent.click(screen.getByRole('button', { name: 'Simpan' }));
    expect(await screen.findByText(/budi@tim.id menyimpan perubahan/)).toBeInTheDocument();
    expect(f).toHaveBeenCalledTimes(2); // tidak ada PUT otomatis lanjutan
    expect(area).toHaveValue('awal saya');
    await userEvent.click(screen.getByRole('button', { name: 'Muat versi terbaru' }));
    expect(screen.getByLabelText('Isi Markdown')).toHaveValue('versi budi');
    await userEvent.type(screen.getByLabelText('Isi Markdown'), '!');
    await userEvent.click(screen.getByRole('button', { name: 'Simpan' }));
    await vi.waitFor(() => expect(f).toHaveBeenCalledTimes(3));
    expect(body(f, 2)).toEqual({ content: 'versi budi!', version: V2 });
  });
  it('T05-AC3: compare then explicit overwrite sends my draft with the latest version', async () => {
    const f = scenario(conflict(), json(200, mockFile('docs/a.md', { content: 'awal saya', version: 'c'.repeat(64) })));
    render(View, props());
    await userEvent.type(await screen.findByLabelText('Isi Markdown'), ' saya');
    await userEvent.click(screen.getByRole('button', { name: 'Simpan' }));
    await userEvent.click(await screen.findByRole('button', { name: 'Bandingkan' }));
    expect(screen.getByText('Versi saya')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Timpa dengan versi saya' }));
    await vi.waitFor(() => expect(f).toHaveBeenCalledTimes(3));
    expect(body(f, 2)).toEqual({ content: 'awal saya', version: V2 });
  });
  it('two consecutive saves use the version from the first response', async () => {
    const v3 = 'c'.repeat(64);
    const f = scenario(json(200, mockFile('docs/a.md', { content: 'awal1', version: v3 })), json(200, mockFile('docs/a.md', { content: 'awal12', version: 'd'.repeat(64) })));
    render(View, props());
    const area = await screen.findByLabelText('Isi Markdown');
    await userEvent.type(area, '1');
    await userEvent.click(screen.getByRole('button', { name: 'Simpan' }));
    await vi.waitFor(() => expect(f).toHaveBeenCalledTimes(2));
    await screen.findByText('Tersimpan');
    await userEvent.type(area, '2');
    await userEvent.click(screen.getByRole('button', { name: 'Simpan' }));
    await vi.waitFor(() => expect(f).toHaveBeenCalledTimes(3));
    expect(body(f, 1).version).toBe(V1);
    expect(body(f, 2).version).toBe(v3);
  });
  it('413 shows the limit and keeps the content', async () => {
    scenario(json(413, { error: { code: 'PAYLOAD_TOO_LARGE', message: 'x', details: { reason: 'too_large', max_bytes: 1048576 } } }));
    render(View, props());
    await userEvent.type(await screen.findByLabelText('Isi Markdown'), 'x');
    await userEvent.click(screen.getByRole('button', { name: 'Simpan' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('1 MB');
    expect(screen.getByLabelText('Isi Markdown')).toHaveValue('awalx');
  });
  it('404 on save keeps the draft, offers save-as-new and notifies', async () => {
    scenario(json(404, { error: { code: 'NOT_FOUND', message: 'x', details: null } }));
    const p = props();
    render(View, p);
    await userEvent.type(await screen.findByLabelText('Isi Markdown'), '!');
    await userEvent.click(screen.getByRole('button', { name: 'Simpan' }));
    await userEvent.click(await screen.findByRole('button', { name: 'Simpan sebagai file baru' }));
    expect(p.onsaveasnew).toHaveBeenCalledWith('awal!');
    expect(p.onnotfound).toHaveBeenCalledWith('docs/a.md');
  });
  it('beforeunload is registered only while dirty', async () => {
    scenario();
    const add = vi.spyOn(window, 'addEventListener');
    render(View, props());
    const area = await screen.findByLabelText('Isi Markdown');
    expect(add.mock.calls.some((c) => c[0] === 'beforeunload')).toBe(false);
    await userEvent.type(area, 'x');
    expect(add.mock.calls.some((c) => c[0] === 'beforeunload')).toBe(true);
  });
  it('cancel while dirty asks for confirmation', async () => {
    scenario();
    const p = props();
    render(View, p);
    await userEvent.type(await screen.findByLabelText('Isi Markdown'), 'x');
    await userEvent.click(screen.getByRole('button', { name: 'Batal' }));
    expect(p.onclose).not.toHaveBeenCalled();
    await userEvent.click(await screen.findByRole('button', { name: 'Buang' }));
    expect(p.onclose).toHaveBeenCalledWith(null);
  });
  it('offers to restore a different saved draft, using its base version', async () => {
    sessionStorage.setItem('grimoire.knowledge.draft:p1:docs/a.md', JSON.stringify({ content: 'draf lama', baseVersion: V2, savedAt: '2026-10-21T09:00:00Z' }));
    const f = scenario(json(200, mockFile('docs/a.md', { content: 'draf lama', version: 'e'.repeat(64) })));
    render(View, props());
    await userEvent.click(await screen.findByRole('button', { name: 'Pulihkan' }));
    expect(screen.getByLabelText('Isi Markdown')).toHaveValue('draf lama');
    await userEvent.click(screen.getByRole('button', { name: 'Simpan' }));
    await vi.waitFor(() => expect(f).toHaveBeenCalledTimes(2));
    expect(body(f, 1).version).toBe(V2);
  });
});

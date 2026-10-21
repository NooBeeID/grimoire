import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { setLocale } from '../../../../../shared/stores/i18n';
import Dialog from './RenameSpecDialog.svelte';

const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status });
const spec = {
  name: 'payment-v2', title: 'Payment API', openapi_version: '3.0.3', api_version: '1.4.0', format: 'yaml', source_path: null,
  size_bytes: 1, endpoint_count: 2, created_at: '2026-10-23T09:00:00Z', created_by_label: null, updated_at: '2026-10-24T10:00:00Z', updated_by_label: null,
};
const props = () => ({
  open: true, projectId: 'p1', spec: { name: 'payment' },
  onclose: vi.fn(), onrenamed: vi.fn(), onspecgone: vi.fn(), onprojectgone: vi.fn(),
});
beforeEach(() => setLocale('id'));
afterEach(() => vi.unstubAllGlobals());

async function type(v: string) {
  const input = screen.getByLabelText('Nama baru');
  await userEvent.clear(input);
  await userEvent.type(input, v);
}

describe('RenameSpecDialog', () => {
  it('save is disabled until the name changes; sends no request for the same name', async () => {
    const f = vi.fn();
    vi.stubGlobal('fetch', f);
    render(Dialog, props());
    expect(screen.getByRole('button', { name: 'Simpan' })).toBeDisabled();
    expect(f).not.toHaveBeenCalled();
  });
  it('renames and notifies', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(200, { spec })));
    const p = props();
    render(Dialog, p);
    await type('payment-v2');
    await userEvent.click(screen.getByRole('button', { name: 'Simpan' }));
    await vi.waitFor(() => expect(p.onrenamed).toHaveBeenCalledWith(expect.objectContaining({ name: 'payment-v2' })));
  });
  it('invalid name -> client error, no request', async () => {
    const f = vi.fn();
    vi.stubGlobal('fetch', f);
    render(Dialog, props());
    await type('Bad Name');
    await userEvent.click(screen.getByRole('button', { name: 'Simpan' }));
    expect(await screen.findAllByText(/Huruf kecil, angka/)).not.toHaveLength(0);
    expect(f).not.toHaveBeenCalled();
  });
  it('409 NAME_TAKEN shows the field error; 404 calls onspecgone', async () => {
    const f = vi
      .fn()
      .mockResolvedValueOnce(json(409, { error: { code: 'CONFLICT', message: 'x', details: { reason: 'NAME_TAKEN' } } }))
      .mockResolvedValueOnce(json(404, { error: { code: 'NOT_FOUND', message: 'x', details: null } }));
    vi.stubGlobal('fetch', f);
    const p = props();
    render(Dialog, p);
    await type('auth');
    await userEvent.click(screen.getByRole('button', { name: 'Simpan' }));
    expect(await screen.findByText('Nama ini sudah dipakai spec lain di project ini')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Simpan' }));
    await vi.waitFor(() => expect(p.onspecgone).toHaveBeenCalled());
  });
});

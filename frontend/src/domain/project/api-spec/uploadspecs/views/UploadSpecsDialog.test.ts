import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { setLocale } from '../../../../../shared/stores/i18n';
import { mockFileResult, mockRejected, mockSwagger2Created, mockUploadResult, mockValidateResult } from '../mocks/results';
import Dialog from './UploadSpecsDialog.svelte';

const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status });
beforeEach(() => setLocale('id'));
afterEach(() => vi.unstubAllGlobals());

const props = (over = {}) => ({
  open: true,
  projectId: 'p1',
  existingNames: new Set<string>(['payment']),
  onclose: vi.fn(),
  onuploaded: vi.fn(),
  onstale: vi.fn(),
  onprojectgone: vi.fn(),
  ...over,
});
const pick = (...names: string[]) =>
  userEvent.setup({ applyAccept: false }).upload(screen.getByTestId('apispec-file-input'), names.map((n) => new File(['x'], n)));

describe('UploadSpecsDialog', () => {
  it('T06-AC1: upload shows results and calls onuploaded once', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(200, mockUploadResult([mockFileResult()]))));
    const p = props();
    render(Dialog, p);
    await pick('payment.yaml');
    await userEvent.click(screen.getByRole('button', { name: 'Unggah' }));
    expect(await screen.findByRole('link', { name: 'payment' })).toBeInTheDocument();
    expect(p.onuploaded).toHaveBeenCalledTimes(1);
  });
  it('T06-AC2: rejected rows show reasons and issue messages', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(200, mockUploadResult([mockRejected()]))));
    render(Dialog, props());
    await pick('legacy.yaml');
    await userEvent.click(screen.getByRole('button', { name: 'Unggah' }));
    expect(await screen.findByText('Dokumen OpenAPI tidak valid')).toBeInTheDocument();
    expect(screen.getByTestId('issues-0')).toHaveTextContent('UNSUPPORTED_VERSION');
  });
  it('T10-AC1: upload Swagger 2.0 tampil created dengan label Swagger 2.0 dan memanggil onuploaded sekali', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(200, mockUploadResult([mockSwagger2Created()]))));
    const p = props();
    render(Dialog, p);
    await pick('petstore.yaml');
    await userEvent.click(screen.getByRole('button', { name: 'Unggah' }));
    expect(await screen.findByRole('link', { name: 'petstore' })).toBeInTheDocument();
    expect(screen.getByText(/Swagger Petstore/)).toHaveTextContent('Swagger Petstore · Swagger 2.0');
    expect(p.onuploaded).toHaveBeenCalledTimes(1);
  });
  it('T10: validasi Swagger 2.0 tampil valid tanpa onuploaded', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(200, mockValidateResult([mockSwagger2Created({ status: 'valid' })]))));
    const p = props();
    render(Dialog, p);
    await pick('petstore.yaml');
    await userEvent.click(screen.getByRole('button', { name: 'Validasi saja' }));
    expect(await screen.findByText(/Swagger 2\.0/)).toBeInTheDocument();
    expect(p.onuploaded).not.toHaveBeenCalled();
  });
  it('validate hits /validate, marks overwrite and does not call onuploaded', async () => {
    const f = vi.fn(async () => json(200, mockValidateResult([mockFileResult({ status: 'valid' })])));
    vi.stubGlobal('fetch', f);
    const p = props();
    render(Dialog, p);
    await pick('payment.yaml');
    await userEvent.click(screen.getByRole('button', { name: 'Validasi saja' }));
    expect(await screen.findByText('Akan menimpa spec yang ada')).toBeInTheDocument();
    expect((f.mock.calls[0] as unknown as [string])[0]).toBe('/api/projects/p1/api-specs/validate');
    expect(p.onuploaded).not.toHaveBeenCalled();
  });
  it('non-spec files are not sent and appear as skipped', async () => {
    const f = vi.fn(async (_u: string, _i?: RequestInit) => json(200, mockUploadResult([mockFileResult()])));
    vi.stubGlobal('fetch', f);
    render(Dialog, props());
    await pick('payment.yaml', 'README.md');
    await userEvent.click(screen.getByRole('button', { name: 'Unggah' }));
    await screen.findByText('Bukan file spec (.yaml/.yml/.json)');
    expect((f.mock.calls[0]![1]!.body as FormData).getAll('files')).toHaveLength(1);
  });
  it('invalid name -> client error without a request; many files disable the name field', async () => {
    const f = vi.fn();
    vi.stubGlobal('fetch', f);
    render(Dialog, props());
    await pick('a.yaml');
    await userEvent.type(screen.getByLabelText('Nama spec (opsional)'), 'Bad Name');
    await userEvent.click(screen.getByRole('button', { name: 'Unggah' }));
    expect(await screen.findByText('Nama spec tidak valid')).toBeInTheDocument();
    expect(f).not.toHaveBeenCalled();
    await pick('a.yaml', 'b.yaml');
    expect(screen.getByLabelText('Nama spec (opsional)')).toBeDisabled();
  });
  it('error mapping: NO_FILES, 413, 404, INVALID_RESPONSE, 500', async () => {
    const env = (code: string, reason?: string, message = 'm') => ({ error: { code, message, details: reason ? { reason } : null }, trace_id: 'tr-1' });
    const p = props();
    const f = vi
      .fn()
      .mockResolvedValueOnce(json(400, env('INVALID_ARGUMENT', 'NO_FILES')))
      .mockResolvedValueOnce(json(413, env('PAYLOAD_TOO_LARGE')))
      .mockResolvedValueOnce(json(200, { results: 1 }))
      .mockResolvedValueOnce(json(500, env('INTERNAL', undefined, 'gagal server')))
      .mockResolvedValueOnce(json(404, env('NOT_FOUND')));
    vi.stubGlobal('fetch', f);
    render(Dialog, p);
    await pick('a.yaml');
    const submit = () => userEvent.click(screen.getByRole('button', { name: 'Unggah' }));
    await submit();
    expect(await screen.findByText('Tidak ada file yang dikirim')).toBeInTheDocument();
    await submit();
    expect(await screen.findByText(/melebihi batas/)).toBeInTheDocument();
    await submit();
    expect(await screen.findByText(/Respons server tidak valid/)).toBeInTheDocument();
    expect(p.onstale).toHaveBeenCalled();
    await submit();
    expect(await screen.findByText('gagal server')).toBeInTheDocument();
    expect(screen.getByText('ID jejak: tr-1')).toBeInTheDocument();
    await submit();
    await vi.waitFor(() => expect(p.onprojectgone).toHaveBeenCalled());
  });
});

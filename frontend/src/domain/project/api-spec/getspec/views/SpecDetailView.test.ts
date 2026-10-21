import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { setLocale } from '../../../../../shared/stores/i18n';
import { mockDetail } from '../mocks/detail';
import View from './SpecDetailView.svelte';

const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status });
beforeEach(() => setLocale('id'));
afterEach(() => vi.unstubAllGlobals());

const props = (over = {}) => ({
  projectId: 'p1',
  name: 'payment',
  hrefForOp: (k: string | null) => '/projects/p1/api-specs/payment' + (k ? '?op=' + encodeURIComponent(k) : ''),
  backHref: '/projects/p1/api-specs',
  ...over,
});

describe('SpecDetailView', () => {
  it('renders header, tag groups with op links and the overview', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(200, mockDetail())));
    render(View, props());
    expect(await screen.findByRole('heading', { name: 'payment' })).toBeInTheDocument();
    expect(screen.getAllByRole('link', { name: /^GET \/payments Daftar/ })[0]).toHaveAttribute('href', expect.stringContaining('?op=GET%20%2Fpayments'));
    expect(screen.getByText('Ringkasan API')).toBeInTheDocument();
  });
  it('selected op renders parameters, request body and responses without any execute button', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(200, mockDetail())));
    render(View, props({ selectedOp: 'GET /payments' }));
    expect(await screen.findByRole('heading', { name: /\/payments/, level: 3 })).toBeInTheDocument();
    expect(screen.getByText('X-Merchant')).toBeInTheDocument();
    expect(screen.getByText('limit')).toBeInTheDocument();
    const codes = screen.getAllByText(/^(200|default)$/).map((e) => e.textContent);
    expect(codes).toEqual(['200', 'default']);
    expect(screen.queryByRole('button', { name: /try|kirim|execute/i })).toBeNull();
  });
  it('schema tree: level 0 open, children lazy, circular marked', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(200, mockDetail())));
    render(View, props({ selectedOp: 'POST /payments' }));
    expect(await screen.findByText('amount')).toBeInTheDocument();
    expect(screen.getByText(/circular/)).toBeInTheDocument();
  });
  it('unknown op shows not-found with link to overview', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(200, mockDetail())));
    render(View, props({ selectedOp: 'GET /nope' }));
    expect(await screen.findByText('Endpoint tidak ada di spec ini')).toBeInTheDocument();
  });
  it('search filters the sidebar and shows an empty message', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(200, mockDetail())));
    render(View, props());
    await screen.findByRole('heading', { name: 'payment' });
    await userEvent.type(screen.getByRole('searchbox'), 'zzz');
    expect(screen.getByText('Tidak ada endpoint yang cocok')).toBeInTheDocument();
  });
  it('404 shows not found with back link; 500 shows retry', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(404, { error: { code: 'NOT_FOUND', message: 'x', details: null } })));
    const { unmount } = render(View, props());
    expect(await screen.findByText('Spec tidak ditemukan')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Kembali ke daftar spec' })).toHaveAttribute('href', '/projects/p1/api-specs');
    unmount();
    vi.stubGlobal('fetch', vi.fn(async () => json(500, { error: { code: 'INTERNAL', message: 'gagal server', details: null } })));
    render(View, props());
    expect(await screen.findByText('gagal server')).toBeInTheDocument();
  });
  it('renders hostile spec text as text and non-http server URLs as plain text', async () => {
    const d = mockDetail();
    (d.document as Record<string, unknown>).servers = [{ url: 'javascript:alert(1)' }];
    (d.document as Record<string, unknown>).info = { title: 'x', version: '1', description: '<img src=x onerror=alert(1)>' };
    vi.stubGlobal('fetch', vi.fn(async () => json(200, d)));
    const { container } = render(View, props());
    await screen.findByText('<img src=x onerror=alert(1)>');
    expect(container.querySelector('img')).toBeNull();
    expect(screen.getByText('javascript:alert(1)').closest('a')).toBeNull();
  });
});

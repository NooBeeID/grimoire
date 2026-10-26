import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { setLocale } from '../../../../../shared/stores/i18n';
import { mockUsageAll, mockUsageSection, PROJECT_ID } from '../mocks/usage';
import UsageInsightsView from './UsageInsightsView.svelte';

let fetchMock: ReturnType<typeof vi.fn>;
function stub(failing: string[] = []) {
  fetchMock = vi.fn(async (url: string) => {
    const section = new URL(url, 'http://x').searchParams.get('section') as 'keys';
    if (failing.includes(section))
      return new Response(JSON.stringify({ error: { code: 'INTERNAL', message: 'gagal server' } }), { status: 500 });
    return new Response(JSON.stringify(mockUsageSection(mockUsageAll, section)), { status: 200 });
  });
  vi.stubGlobal('fetch', fetchMock);
}
const urls = () => fetchMock.mock.calls.map((c) => String(c[0]));
beforeEach(() => setLocale('id'));
afterEach(() => vi.unstubAllGlobals());

describe('UsageInsightsView', () => {
  it('no project: no request for top_files/unread_files', async () => {
    stub();
    render(UsageInsightsView, { query: new URLSearchParams() });
    expect(await screen.findByText('ci-review-agent')).toBeInTheDocument();
    expect(urls().some((u) => u.includes('section=top_files') || u.includes('section=unread_files'))).toBe(false);
    expect(screen.getAllByText('Pilih project')).toHaveLength(2);
  });
  it('with project: four requests, one failing keeps others', async () => {
    stub(['keys']);
    render(UsageInsightsView, { query: new URLSearchParams(`project=${PROJECT_ID}`) });
    expect(await screen.findByText('docs/payments/refund-flow.md')).toBeInTheDocument();
    expect(await screen.findByText('gagal server')).toBeInTheDocument();
    expect(urls()).toHaveLength(4);
  });
  it('switching tab shows zero queries table', async () => {
    stub();
    render(UsageInsightsView, { query: new URLSearchParams() });
    await userEvent.click(screen.getByRole('tab', { name: 'Pencarian' }));
    expect(await screen.findByText('idempotency key retry')).toBeInTheDocument();
  });
  it('re-loads on query change', async () => {
    stub();
    const { rerender } = render(UsageInsightsView, { query: new URLSearchParams() });
    await screen.findByText('ci-review-agent');
    await rerender({ query: new URLSearchParams('range=30d') });
    await vi.waitFor(() => expect(urls().some((u) => u.includes('range=30d'))).toBe(true));
  });
});

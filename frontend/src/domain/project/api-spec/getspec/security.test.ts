import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { setLocale } from '../../../../shared/stores/i18n';
import { mockDetailXss } from './mocks/detail-xss';
import View from './views/SpecDetailView.svelte';

const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status });
const props = (over = {}) => ({
  projectId: 'p1',
  name: 'evil',
  hrefForOp: (k: string | null) => '/projects/p1/api-specs/evil' + (k ? '?op=' + encodeURIComponent(k) : ''),
  backHref: '/projects/p1/api-specs',
  ...over,
});

function expectClean(root: HTMLElement) {
  expect(root.querySelector('script')).toBeNull();
  expect(root.querySelector('img')).toBeNull();
  for (const el of root.querySelectorAll('*')) {
    for (const a of el.attributes) {
      expect(a.name.toLowerCase().startsWith('on'), `${el.tagName}[${a.name}]`).toBe(false);
    }
  }
  for (const a of root.querySelectorAll('a')) {
    expect((a.getAttribute('href') ?? '').trim().toLowerCase().startsWith('javascript:')).toBe(false);
  }
}

let alertSpy: ReturnType<typeof vi.fn>;
beforeEach(() => {
  setLocale('id');
  alertSpy = vi.fn();
  vi.stubGlobal('alert', alertSpy);
  vi.stubGlobal('fetch', vi.fn(async () => json(200, mockDetailXss())));
});
afterEach(() => vi.unstubAllGlobals());

describe('hostile spec content (T06-AC8)', () => {
  it('overview: description, tags, servers, links and schemes stay inert', async () => {
    const { container } = render(View, props());
    await screen.findByText('Evil · 1');
    // Markdown dirender: HTML mentah menjadi teks, tautan aman dipertahankan.
    expect(screen.getAllByText(/<script>alert\(1\)<\/script>/).length).toBeGreaterThan(0);
    const ok = screen.getByRole('link', { name: 'ok' });
    expect(ok).toHaveAttribute('href', 'https://example.com/ok');
    expect(ok).toHaveAttribute('rel', expect.stringContaining('noopener'));
    for (const el of screen.getAllByText('klik')) expect(el.closest('a')).toBeNull();
    // URL non-http tampil sebagai teks.
    for (const el of screen.getAllByText('javascript:alert(1)')) expect(el.closest('a')).toBeNull();
    // contact/license/termsOfService non-http tidak pernah menjadi tautan.
    expect(container.querySelector('a[href*="alert"]')).toBeNull();
    expectClean(container);
    expect(alertSpy).not.toHaveBeenCalled();
  });

  it('operation: summary, description, parameter, response, schema, examples and security stay inert', async () => {
    const { container } = render(View, props({ selectedOp: 'GET /x' }));
    await screen.findByRole('heading', { level: 3, name: /\/x/ });
    await userEvent.click(screen.getAllByRole('button', { name: /Salin|Copy/ })[0]!);
    expectClean(container);
    expect(screen.getAllByText(/onerror=alert\(1\)/).length).toBeGreaterThan(0);
    expect(alertSpy).not.toHaveBeenCalled();
  });
});

import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { mockDetail } from '../mocks/project';
import type { ProjectContext } from '../types';
import Harness from './ProjectContextHarness.svelte';

afterEach(() => vi.unstubAllGlobals());
function stub(responses: Array<[number, unknown]>) {
  let i = 0;
  const f = vi.fn(async (_u: string, _init?: RequestInit) => {
    const [s, b] = responses[Math.min(i++, responses.length - 1)]!;
    return new Response(JSON.stringify(b), { status: s });
  });
  vi.stubGlobal('fetch', f);
  return f;
}
const D = mockDetail();

describe('ProjectContextView', () => {
  it('T03-AC1: 404 shows uniform not found with link home', async () => {
    stub([[404, { error: { code: 'NOT_FOUND', message: 'project tidak ditemukan', details: null } }]]);
    render(Harness, { projectId: 'nope', capture: vi.fn() });
    expect(await screen.findByText('Project tidak ditemukan')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Kembali ke daftar project' })).toHaveAttribute('href', '/');
  });
  it('ready renders children with ctx.project', async () => {
    stub([[200, D]]);
    render(Harness, { projectId: D.id, capture: vi.fn() });
    expect(await screen.findByTestId('child')).toHaveTextContent('child:Payments');
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('Payments');
  });
  it('reload refetches keeping content; replace updates header without fetch', async () => {
    const f = stub([[200, D], [200, { ...D, name: 'Reloaded' }]]);
    let ctx!: ProjectContext;
    render(Harness, { projectId: D.id, capture: (c: ProjectContext) => (ctx = c) });
    await screen.findByTestId('child');
    const p = ctx.reload();
    expect(screen.getByTestId('child')).toBeInTheDocument();
    await p;
    expect(f).toHaveBeenCalledTimes(2);
    expect(await screen.findByText('child:Reloaded')).toBeInTheDocument();
    ctx.replace({ ...ctx.project, name: 'Replaced' });
    expect(await screen.findByText('child:Replaced')).toBeInTheDocument();
    expect(f).toHaveBeenCalledTimes(2);
  });
  it('500 shows ErrorState with retry', async () => {
    const f = stub([[500, { error: { code: 'INTERNAL', message: 'boom', details: null } }], [200, D]]);
    render(Harness, { projectId: D.id, capture: vi.fn() });
    expect(await screen.findByText('boom')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Coba lagi' }));
    expect(await screen.findByTestId('child')).toBeInTheDocument();
    expect(f).toHaveBeenCalledTimes(2);
  });
});

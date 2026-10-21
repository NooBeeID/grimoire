import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '../../../../../shared/services/http';
import { setLocale } from '../../../../../shared/stores/i18n';
import { mockSummary } from '../mocks/entries';
import { ACTION_GROUPS, AUDIT_ACTIONS, EMPTY_FILTER, type AuditFilter } from '../types';
import ActionPicker from './ActionPicker.svelte';
import AuditTable from './AuditTable.svelte';
import FilterBar from './FilterBar.svelte';

beforeEach(() => setLocale('id'));
const noop = () => {};
const baseProps = () => ({
  rows: [mockSummary(1), mockSummary(2, { actor_type: 'user', actor_label: 'budi' })],
  status: 'ready' as const,
  error: null,
  selectedId: null as string | null,
  hasMore: false,
  moreStatus: 'idle' as const,
  moreError: null,
  filtered: false,
  onselect: noop,
  onloadmore: noop,
  onretry: noop,
  onretrymore: noop,
  onresetfilter: noop,
});
const apiErr = (status: number) => new ApiError({ status, code: 'X', message: 'gagal', traceId: 'tr-9' });

describe('AuditTable', () => {
  it('T02-AC1 shows agent and user icons with accessible names, i18n columns', () => {
    render(AuditTable, baseProps());
    expect(screen.getByRole('img', { name: 'API key (agent)' })).toBeInTheDocument();
    expect(screen.getByRole('img', { name: 'Pengguna' })).toBeInTheDocument();
    expect(screen.getByRole('columnheader', { name: 'Aktor' })).toBeInTheDocument();
  });
  it('row without target shows a dash and keeps the agent icon name', () => {
    const rows = [mockSummary(1, { action: 'mcp.tools_list', target_type: null, target: null })];
    const { container } = render(AuditTable, { ...baseProps(), rows });
    expect(screen.getByRole('img', { name: 'API key (agent)' })).toBeInTheDocument();
    expect(container.textContent).toContain('mcp.tools_list');
    expect(container.textContent).toContain('mcp.tools_list-Berhasil');
  });
  it('renders XSS actor_label as text', () => {
    const x = '<img src=x onerror=alert(1)>';
    const { container } = render(AuditTable, { ...baseProps(), rows: [mockSummary(1, { actor_label: x })] });
    expect(container.querySelector('img')).toBeNull();
    expect(container.textContent).toContain(x);
  });
  it('click and Enter on time button call onselect; selected gets aria-current', async () => {
    const onselect = vi.fn();
    const p = baseProps();
    const { container } = render(AuditTable, { ...p, onselect, selectedId: p.rows[0]!.id });
    const btns = container.querySelectorAll<HTMLButtonElement>('button[data-entry-id]');
    await userEvent.click(btns[0]!);
    btns[1]!.focus();
    await userEvent.keyboard('{Enter}');
    expect(onselect).toHaveBeenNthCalledWith(1, p.rows[0]!.id);
    expect(onselect).toHaveBeenNthCalledWith(2, p.rows[1]!.id);
    expect(container.querySelector('[aria-current="true"]')).not.toBeNull();
  });
  it('empty without/with filter', async () => {
    const a = render(AuditTable, { ...baseProps(), rows: [] });
    expect(screen.getByText('Belum ada entri audit')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Reset' })).toBeNull();
    a.unmount();
    const onresetfilter = vi.fn();
    render(AuditTable, { ...baseProps(), rows: [], filtered: true, onresetfilter });
    await userEvent.click(screen.getByRole('button', { name: 'Reset' }));
    expect(onresetfilter).toHaveBeenCalled();
  });
  it('403 has no retry; 500 has retry and trace id', () => {
    const a = render(AuditTable, { ...baseProps(), error: apiErr(403), rows: [] });
    expect(screen.queryByRole('button', { name: 'Coba lagi' })).toBeNull();
    a.unmount();
    render(AuditTable, { ...baseProps(), error: apiErr(500), status: 'error', rows: [] });
    expect(screen.getByRole('button', { name: 'Coba lagi' })).toBeInTheDocument();
    expect(screen.getByText('ID jejak: tr-9')).toBeInTheDocument();
  });
  it('load more only with hasMore, disabled while loading, inline error with retry, end of list', async () => {
    const a = render(AuditTable, baseProps());
    expect(screen.queryByRole('button', { name: /Muat berikutnya/ })).toBeNull();
    expect(screen.getByText('Semua entri sudah dimuat')).toBeInTheDocument();
    a.unmount();
    const b = render(AuditTable, { ...baseProps(), hasMore: true, moreStatus: 'loading' });
    expect(screen.getByRole('button', { name: /Muat berikutnya/ })).toBeDisabled();
    b.unmount();
    const onretrymore = vi.fn();
    render(AuditTable, { ...baseProps(), hasMore: true, moreStatus: 'error', moreError: apiErr(500), onretrymore });
    await userEvent.click(screen.getByRole('button', { name: 'Coba lagi' }));
    expect(onretrymore).toHaveBeenCalled();
  });
});

describe('ActionPicker', () => {
  it('20 selected disables the rest; search filters; unknown goes to Lainnya', async () => {
    const twenty = AUDIT_ACTIONS.slice(0, 20);
    const a = render(ActionPicker, { value: twenty, onchange: noop });
    const boxes = a.container.querySelectorAll<HTMLInputElement>('input[type=checkbox]');
    expect([...boxes].filter((b) => !b.checked).every((b) => b.disabled)).toBe(true);
    a.unmount();
    const b = render(ActionPicker, { value: ['custom.thing'], onchange: noop });
    expect(screen.getByText('Lainnya')).toBeInTheDocument();
    await userEvent.type(screen.getByRole('searchbox'), 'read_file');
    expect(b.container.querySelectorAll('input[type=checkbox]')).toHaveLength(1);
  });
  it('searching unknown shows only mcp.unknown_tool and checking it reports it', async () => {
    const onchange = vi.fn();
    const r = render(ActionPicker, { value: [], onchange });
    await userEvent.type(screen.getByRole('searchbox'), 'unknown');
    expect(r.container.querySelectorAll('input[type=checkbox]')).toHaveLength(1);
    await userEvent.click(screen.getByRole('checkbox'));
    expect(onchange).toHaveBeenCalledWith(['mcp.unknown_tool']);
  });
});

describe('FilterBar', () => {
  const suggestions = {
    actors: [{ actorType: 'api_key' as const, actorId: '0192f0c4-7a1e-7c3a-9b1d-3c4e5f6a7b8c', actorLabel: 'claude' }],
    projects: [{ projectId: '0192f0c4-0000-7000-8000-000000000001', projectLabel: 'Docs' }],
  };
  const value = (): AuditFilter => ({ ...EMPTY_FILTER, actions: [], statuses: [] });

  it('apply yields filter with range preset', async () => {
    const onapply = vi.fn();
    render(FilterBar, { value: value(), suggestions, onapply, onreset: noop });
    await userEvent.selectOptions(screen.getByLabelText('Rentang waktu'), '7d');
    await userEvent.click(screen.getByRole('button', { name: 'Terapkan' }));
    expect(onapply.mock.calls[0]![0]).toMatchObject({ range: '7d' });
  });
  it('actor suggestion fills type and id; chip removable', async () => {
    const onapply = vi.fn();
    render(FilterBar, { value: value(), suggestions, onapply, onreset: noop });
    await userEvent.type(screen.getByLabelText('Aktor'), 'claude');
    expect(screen.getByText('ID aktor: 0192f0c4')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Hapus filter ID aktor' }));
    expect(screen.queryByText('ID aktor: 0192f0c4')).toBeNull();
  });
  it('free-text project errors and blocks apply; target has maxlength; labels exist; reset', async () => {
    const onapply = vi.fn();
    const onreset = vi.fn();
    render(FilterBar, { value: value(), suggestions, onapply, onreset });
    await userEvent.type(screen.getByLabelText('Project'), 'bukan uuid');
    await userEvent.click(screen.getByRole('button', { name: 'Terapkan' }));
    expect(onapply).not.toHaveBeenCalled();
    expect(screen.getByText(/tempel UUID project/)).toBeInTheDocument();
    expect(screen.getByLabelText('Teks target')).toHaveAttribute('maxlength', '200');
    expect(screen.getByLabelText('Jenis aktor')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Reset' }));
    expect(onreset).toHaveBeenCalled();
  });
});

describe('katalog aksi T08', () => {
  it('grup mcp memuat tiga aksi knowledge-tools', () => {
    const mcp = ACTION_GROUPS.find((g) => g.key === 'mcp');
    expect(mcp?.actions).toEqual(expect.arrayContaining(['mcp.list_files', 'mcp.read_file', 'mcp.search_knowledge']));
  });
});

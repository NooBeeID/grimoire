import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { mockCandidate } from '../mocks/members';
import type { CandidateState } from '../stores/candidateSearch.store';
import CandidateCombobox from './CandidateCombobox.svelte';

const st = (over: Partial<CandidateState> = {}): CandidateState => ({
  q: 'bud',
  status: 'ready',
  items: [mockCandidate(1), mockCandidate(2)],
  error: null,
  activeIndex: -1,
  inviteOption: null,
  ...over,
});
const cbs = () => ({ oninput: vi.fn(), onmove: vi.fn(), onselect: vi.fn(), onclear: vi.fn() });

describe('CandidateCombobox', () => {
  it('exposes combobox ARIA, opens on input and tracks active option', async () => {
    const { rerender } = render(CandidateCombobox, { search: st({ q: '' , status: 'idle', items: [] }), ...cbs() });
    const input = screen.getByRole('combobox');
    expect(input).toHaveAttribute('aria-autocomplete', 'list');
    expect(input).toHaveAttribute('aria-expanded', 'false');
    await userEvent.click(input);
    await rerender({ search: st({ activeIndex: 1 }), ...cbs() });
    expect(input).toHaveAttribute('aria-expanded', 'true');
    const opts = screen.getAllByRole('option');
    expect(opts).toHaveLength(2);
    expect(input.getAttribute('aria-activedescendant')).toBe(opts[1]!.id);
    expect(opts[1]).toHaveAttribute('aria-selected', 'true');
  });
  it('ArrowDown moves, Enter selects the active option', async () => {
    const c = cbs();
    render(CandidateCombobox, { search: st({ activeIndex: 0 }), ...c });
    const input = screen.getByRole('combobox');
    await userEvent.click(input);
    await userEvent.keyboard('{ArrowDown}');
    expect(c.onmove).toHaveBeenCalledWith(1);
    await userEvent.keyboard('{Enter}');
    expect(c.onselect).toHaveBeenCalledWith(expect.objectContaining({ name: 'Budi 1' }));
  });
  it('Escape closes the list', async () => {
    render(CandidateCombobox, { search: st(), ...cbs() });
    const input = screen.getByRole('combobox');
    await userEvent.click(input);
    expect(input).toHaveAttribute('aria-expanded', 'true');
    await userEvent.keyboard('{Escape}');
    expect(input).toHaveAttribute('aria-expanded', 'false');
  });
  it('error state shows searchError, not noCandidates', async () => {
    render(CandidateCombobox, { search: st({ status: 'error', items: [] }), ...cbs() });
    await userEvent.click(screen.getByRole('combobox'));
    expect(screen.getByText('Pencarian gagal. Coba lagi.')).toBeInTheDocument();
    expect(screen.queryByText('Tidak ada developer aktif yang cocok')).toBeNull();
  });
  it('empty ready state shows noCandidates; loading shows searching', async () => {
    const { rerender } = render(CandidateCombobox, { search: st({ items: [] }), ...cbs() });
    await userEvent.click(screen.getByRole('combobox'));
    expect(screen.getByText('Tidak ada developer aktif yang cocok')).toBeInTheDocument();
    await rerender({ search: st({ status: 'loading', items: [] }), ...cbs() });
    expect(screen.getByText('Mencari...')).toBeInTheDocument();
  });
});

describe('CandidateCombobox: opsi undang', () => {
  const inv = (over: Partial<CandidateState> = {}) => st({ q: 'baru@tim.id', items: [], inviteOption: 'invite', ...over });
  it('menampilkan opsi undang saat nol kandidat; Enter setelah ArrowDown memanggil oninvite', async () => {
    const c = { ...cbs(), oninvite: vi.fn() };
    const { rerender } = render(CandidateCombobox, { search: st({ q: '', status: 'idle', items: [] }), ...c });
    await userEvent.type(screen.getByRole('combobox'), 'b');
    await rerender({ search: inv(), ...c });
    const opt = await screen.findByTestId('invite-option');
    expect(opt).toHaveTextContent('Undang baru@tim.id lewat email');
    expect(screen.getByText('Tidak ada developer aktif yang cocok')).toBeInTheDocument();
    await rerender({ search: inv({ activeIndex: 0 }), ...c });
    expect(screen.getByRole('combobox')).toHaveAttribute('aria-activedescendant', opt.id);
    await userEvent.keyboard('{Enter}');
    expect(c.oninvite).toHaveBeenCalledWith('baru@tim.id');
  });
  it('mousedown pada opsi memanggil oninvite', async () => {
    const c = { ...cbs(), oninvite: vi.fn() };
    const { rerender } = render(CandidateCombobox, { search: st({ q: '', status: 'idle', items: [] }), ...c });
    await userEvent.type(screen.getByRole('combobox'), 'b');
    await rerender({ search: inv({ items: [mockCandidate(1)] }), ...c });
    await userEvent.click(await screen.findByTestId('invite-option'));
    expect(c.oninvite).toHaveBeenCalledWith('baru@tim.id');
  });
  it('inviteOption member menampilkan teks tanpa opsi', async () => {
    const c = { ...cbs(), oninvite: vi.fn() };
    const { rerender } = render(CandidateCombobox, { search: st({ q: '', status: 'idle', items: [] }), ...c });
    await userEvent.type(screen.getByRole('combobox'), 'b');
    await rerender({ search: inv({ inviteOption: 'member' }), ...c });
    expect(await screen.findByText('Email ini sudah menjadi anggota')).toBeInTheDocument();
    expect(screen.queryByTestId('invite-option')).toBeNull();
  });
  it('kandidat invitation_pending diberi penanda; error pencarian tanpa opsi undang', async () => {
    const c = { ...cbs(), oninvite: vi.fn() };
    const { rerender } = render(CandidateCombobox, { search: st({ q: '', status: 'idle', items: [] }), ...c });
    await userEvent.type(screen.getByRole('combobox'), 'b');
    await rerender({ search: st({ items: [mockCandidate(1, { invitation_pending: true })] }), ...c });
    expect(await screen.findByText('undangan menunggu')).toBeInTheDocument();
    await rerender({ search: inv({ status: 'error', inviteOption: null }), ...c });
    expect(screen.queryByTestId('invite-option')).toBeNull();
  });
});

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

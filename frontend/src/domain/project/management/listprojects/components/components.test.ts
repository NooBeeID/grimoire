import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { mockProject } from '../mocks/projects';
import ProjectSearchBar from './ProjectSearchBar.svelte';
import ProjectTable from './ProjectTable.svelte';

describe('ProjectTable', () => {
  it('shows role as text, not-member badge for admin (T03-AC7), links and literal names', () => {
    const items = [
      mockProject(1, { my_role: 'owner', access_level: 'owner', name: 'Alpha' }),
      mockProject(2, { name: 'Beta' }),
      mockProject(3, { my_role: null, access_level: 'admin', is_member: false, name: 'Gamma' }),
      mockProject(4, { name: '<img src=x onerror=alert(1)>' }),
    ];
    const { container } = render(ProjectTable, { items });
    expect(screen.getByText('Owner')).toBeInTheDocument();
    expect(screen.getAllByText('Member').length).toBeGreaterThan(0);
    expect(screen.getByText('Bukan anggota')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Alpha' })).toHaveAttribute('href', `/projects/${items[0]!.id}`);
    expect(screen.getByText('<img src=x onerror=alert(1)>')).toBeInTheDocument();
    expect(container.querySelector('img')).toBeNull();
    expect(container.querySelector(`[data-project-id="${items[0]!.id}"]`)).not.toBeNull();
  });
  it('drops the file/spec columns while no row has a count', () => {
    render(ProjectTable, { items: [mockProject(1)] });
    expect(screen.queryByText('File')).toBeNull();
    expect(screen.queryByText('Spec')).toBeNull();
  });
  it('shows a count column once any row has a value', () => {
    const counts = { files: 12, specs: null, keys: null };
    render(ProjectTable, { items: [mockProject(1, { counts }), mockProject(2)] });
    expect(screen.getByText('File')).toBeInTheDocument();
    expect(screen.getByText('12')).toBeInTheDocument();
    expect(screen.queryByText('Spec')).toBeNull();
  });
});

describe('ProjectSearchBar', () => {
  beforeEach(() => vi.useFakeTimers({ shouldAdvanceTime: true }));
  afterEach(() => vi.useRealTimers());

  it('debounces typing 300ms to one call', async () => {
    const onsearch = vi.fn();
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    render(ProjectSearchBar, { value: '', onsearch });
    await user.type(screen.getByRole('searchbox'), 'pay');
    expect(onsearch).not.toHaveBeenCalled();
    vi.advanceTimersByTime(300);
    expect(onsearch).toHaveBeenCalledTimes(1);
    expect(onsearch).toHaveBeenCalledWith('pay');
  });
  it('Enter searches immediately; clear button resets', async () => {
    const onsearch = vi.fn();
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    render(ProjectSearchBar, { value: '', onsearch });
    await user.type(screen.getByRole('searchbox'), 'a{Enter}');
    expect(onsearch).toHaveBeenCalledWith('a');
    await user.click(screen.getByRole('button', { name: 'Hapus pencarian' }));
    expect(onsearch).toHaveBeenLastCalledWith('');
  });
});

import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import UsersFilterBar from './UsersFilterBar.svelte';

describe('UsersFilterBar', () => {
  it('emits partial changes and reset', async () => {
    const onchange = vi.fn();
    render(UsersFilterBar, { filters: { role: '', status: '', q: '' }, hasFilters: true, onchange });
    await userEvent.selectOptions(screen.getByLabelText('Role'), 'developer');
    expect(onchange).toHaveBeenCalledWith({ role: 'developer' });
    await userEvent.selectOptions(screen.getByLabelText('Status'), 'inactive');
    expect(onchange).toHaveBeenCalledWith({ status: 'inactive' });
    await userEvent.type(screen.getByLabelText('Cari nama atau email'), 'a');
    expect(onchange).toHaveBeenCalledWith({ q: 'a' });
    await userEvent.click(screen.getByRole('button', { name: 'Reset filter' }));
    expect(onchange).toHaveBeenLastCalledWith({ role: '', status: '', q: '' });
  });
  it('hides reset without filters', () => {
    render(UsersFilterBar, { filters: { role: '', status: '', q: '' }, onchange: vi.fn() });
    expect(screen.queryByRole('button', { name: 'Reset filter' })).toBeNull();
  });
});


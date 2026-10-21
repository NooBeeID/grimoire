import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import DangerZone from './DangerZone.svelte';
import DeleteProjectForm from './DeleteProjectForm.svelte';

const project = { id: 'x', name: 'Payments', member_count: 3, counts: { files: null, specs: 2, keys: null } };

describe('DangerZone', () => {
  it('button calls onrequest', async () => {
    const onrequest = vi.fn();
    render(DangerZone, { onrequest });
    await userEvent.click(screen.getByRole('button', { name: 'Hapus project' }));
    expect(onrequest).toHaveBeenCalled();
  });
});

describe('DeleteProjectForm', () => {
  it('T03-AC6: enabled only when name is exact', async () => {
    render(DeleteProjectForm, { project, onsubmit: vi.fn(), oncancel: vi.fn() });
    const btn = screen.getByRole('button', { name: 'Hapus permanen' });
    const input = screen.getByLabelText('Nama project');
    expect(btn).toBeDisabled();
    await userEvent.type(input, 'payments');
    expect(btn).toBeDisabled();
    await userEvent.clear(input);
    await userEvent.type(input, 'Payments');
    expect(btn).toBeEnabled();
  });
  it('impact shows dash for null counts', () => {
    render(DeleteProjectForm, { project, onsubmit: vi.fn(), oncancel: vi.fn() });
    expect(screen.getByText('Akan dihapus: 3 anggota, - file, 2 spec, - key.')).toBeInTheDocument();
  });
});

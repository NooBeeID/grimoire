import { render, screen } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import { mockDetail } from '../mocks/project';
import ProjectHeader from './ProjectHeader.svelte';

describe('ProjectHeader', () => {
  it('shows not-member badge for admin non member', () => {
    render(ProjectHeader, { project: mockDetail({ my_role: null, access_level: 'admin', is_member: false }) });
    expect(screen.getByText('Bukan anggota')).toBeInTheDocument();
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('Payments');
  });
  it('multi-line description is text, no html', () => {
    const { container } = render(ProjectHeader, { project: mockDetail({ description: 'a\n<b>x</b>' }) });
    expect(container.querySelector('b')).toBeNull();
    expect(container.querySelector('p')!.className).toContain('whitespace-pre-line');
    expect(screen.getByText('Owner')).toBeInTheDocument();
  });
});

import { render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it } from 'vitest';
import { setLocale } from '../../../../../shared/stores/i18n';
import { mockAuthSpec, mockSpec } from '../mocks/specs';
import OpenApiBadge from '../../../../../shared/components/OpenApiBadge.svelte';
import SpecQuota from './SpecQuota.svelte';
import SpecTable from './SpecTable.svelte';

beforeEach(() => setLocale('id'));

describe('SpecTable', () => {
  it('renders link, badges, endpoint count, source path and updater', () => {
    render(SpecTable, { items: [mockAuthSpec(), mockSpec()], projectId: 'p1' });
    expect(screen.getByRole('link', { name: 'payment' })).toHaveAttribute('href', '/projects/p1/api-specs/payment');
    expect(screen.getByText('3.0')).toBeInTheDocument();
    expect(screen.getByText('3.1')).toBeInTheDocument();
    expect(screen.getByText('specs/payment.yaml')).toBeInTheDocument();
    expect(screen.getByText('Sari')).toBeInTheDocument();
  });
  it('shows an en dash without source_path and renders titles as text', () => {
    render(SpecTable, { items: [mockSpec({ source_path: null, title: '<b>x</b>' })], projectId: 'p1' });
    expect(screen.getByText('–')).toBeInTheDocument();
    expect(screen.getByText('<b>x</b>')).toBeInTheDocument();
  });
});
describe('OpenApiBadge / SpecQuota', () => {
  it('badge has an accessible name', () => {
    render(OpenApiBadge, { version: '3.1.0' });
    expect(screen.getByLabelText('Versi OpenAPI 3.1.0')).toHaveTextContent('3.1');
  });
  it('quota warns at the limit only', () => {
    const { unmount } = render(SpecQuota, { total: 3, limit: 100 });
    expect(screen.getByText('3 / 100 spec')).toBeInTheDocument();
    expect(screen.queryByRole('status')).toBeNull();
    unmount();
    render(SpecQuota, { total: 100, limit: 100 });
    expect(screen.getByRole('status')).toBeInTheDocument();
  });
});

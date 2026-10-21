import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { setLocale } from '../../../../../shared/stores/i18n';
import { mockFileResult, mockRejected } from '../mocks/results';
import FilePicker from './FilePicker.svelte';
import ResultSummary from './ResultSummary.svelte';
import ResultTable from './ResultTable.svelte';

beforeEach(() => setLocale('id'));
afterEach(() => vi.unstubAllGlobals());

describe('FilePicker', () => {
  it('folder input has webkitdirectory and selection calls onpick with relative paths', async () => {
    const onpick = vi.fn();
    render(FilePicker, { specCount: 0, skippedCount: 0, onpick });
    const folder = screen.getByTestId('apispec-folder-input') as HTMLInputElement;
    expect(folder.hasAttribute('webkitdirectory')).toBe(true);
    const file = new File(['x'], 'payment.yaml');
    await userEvent.upload(screen.getByTestId('apispec-file-input'), file);
    expect(onpick).toHaveBeenCalledWith([{ file, path: 'payment.yaml' }]);
  });
  it('shows counts, or the no-spec message', () => {
    const { unmount } = render(FilePicker, { specCount: 2, skippedCount: 1, onpick: vi.fn() });
    expect(screen.getByText('2 file spec dipilih, 1 file lain dilewati')).toBeInTheDocument();
    unmount();
    render(FilePicker, { specCount: 0, skippedCount: 1, onpick: vi.fn() });
    expect(screen.getByText('Tidak ada file .yaml, .yml, atau .json')).toBeInTheDocument();
  });
});
describe('ResultSummary', () => {
  it('lists only statuses above zero with text labels', () => {
    render(ResultSummary, { results: [mockFileResult(), mockRejected()] });
    expect(screen.getByText('Dibuat')).toBeInTheDocument();
    expect(screen.getByText('Ditolak')).toBeInTheDocument();
    expect(screen.queryByText('Dilewati')).toBeNull();
  });
});
describe('ResultTable', () => {
  it('shows reason label, full server issue message and copies the issue text', async () => {
    const writeText = vi.fn(async () => {});
    vi.stubGlobal('navigator', { clipboard: { writeText } });
    render(ResultTable, { results: [mockRejected()] });
    expect(screen.getByText('Dokumen OpenAPI tidak valid')).toBeInTheDocument();
    expect(screen.getByTestId('issues-0')).toHaveTextContent('(baris 1, #/swagger)');
    await userEvent.click(screen.getByRole('button', { name: 'Salin' }));
    expect(writeText).toHaveBeenCalledWith(expect.stringContaining('NOT_OPENAPI_3'));
    expect(await screen.findByText('Tersalin')).toBeInTheDocument();
  });
  it('failed copy shows the manual hint', async () => {
    vi.stubGlobal('navigator', {});
    render(ResultTable, { results: [mockRejected()] });
    await userEvent.click(screen.getByRole('button', { name: 'Salin' }));
    expect(await screen.findByText(/Tidak bisa menyalin otomatis/)).toBeInTheDocument();
  });
  it('links names only after upload and renders hostile values as text', () => {
    const { unmount } = render(ResultTable, { results: [mockFileResult()], projectId: 'p1', linkNames: true });
    expect(screen.getByRole('link', { name: 'payment' })).toHaveAttribute('href', '/projects/p1/api-specs/payment');
    unmount();
    render(ResultTable, { results: [mockRejected({ reason_code: null, reason: '<img src=x onerror=alert(1)>', source_path: '<b>f</b>' })], projectId: 'p1' });
    expect(screen.queryByRole('link')).toBeNull();
    expect(screen.getByText('<img src=x onerror=alert(1)>')).toBeInTheDocument();
    expect(screen.getByText('<b>f</b>')).toBeInTheDocument();
  });
  it('marks overwrite for validated rows', () => {
    render(ResultTable, { results: [{ ...mockFileResult({ status: 'valid' }), willOverwrite: true }] });
    expect(screen.getByText('Akan menimpa spec yang ada')).toBeInTheDocument();
  });
});

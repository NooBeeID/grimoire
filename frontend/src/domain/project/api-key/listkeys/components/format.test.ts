import { beforeEach, describe, expect, it } from 'vitest';
import { setLocale } from '../../../../../shared/stores/i18n';
import { mockKey, mockRevokedKey } from '../mocks/keys';
import { formatDateTime } from '../../../../../shared/utils/datetime';
import { createdAtLine, revokedLine } from './format';

beforeEach(() => setLocale('id'));

describe('format', () => {
  it('createdAtLine returns the date only for both locales', () => {
    const k = mockKey(1, { created_by_label: 'budi@tim.id' });
    expect(createdAtLine(k, 'id')).toBe(formatDateTime(k.created_at, 'id'));
    expect(createdAtLine(k, 'en')).toBe(formatDateTime(k.created_at, 'en'));
    expect(createdAtLine(k, 'id')).not.toContain('budi@tim.id');
  });
  it('revokedLine names the revoker', () => {
    expect(revokedLine(mockRevokedKey(2), 'id')).toMatch(/^Dicabut oleh ana@tim\.id pada /);
  });
  it('revokedLine falls back for unknown revoker and missing time', () => {
    const line = revokedLine(mockRevokedKey(2, { revoked_by_label: null, revoked_at: null }), 'id');
    expect(line).toBe('Dicabut oleh Tidak diketahui pada -');
  });
});

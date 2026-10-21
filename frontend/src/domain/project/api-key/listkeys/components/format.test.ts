import { beforeEach, describe, expect, it } from 'vitest';
import { setLocale } from '../../../../../shared/stores/i18n';
import { mockKey, mockRevokedKey } from '../mocks/keys';
import { createdLine, revokedLine } from './format';

beforeEach(() => setLocale('id'));

describe('format', () => {
  it('createdLine uses creator label', () => {
    const line = createdLine(mockKey(1, { created_by_label: 'budi@tim.id' }), 'id');
    expect(line).toContain('budi@tim.id');
    expect(line).toContain(' · ');
  });
  it('createdLine falls back when creator unknown', () => {
    expect(createdLine(mockKey(1, { created_by_label: null }), 'id')).toContain('Tidak diketahui');
  });
  it('revokedLine names the revoker', () => {
    expect(revokedLine(mockRevokedKey(2), 'id')).toMatch(/^Dicabut oleh ana@tim\.id pada /);
  });
  it('revokedLine falls back for unknown revoker and missing time', () => {
    const line = revokedLine(mockRevokedKey(2, { revoked_by_label: null, revoked_at: null }), 'id');
    expect(line).toBe('Dicabut oleh Tidak diketahui pada -');
  });
});

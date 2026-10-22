import { describe, expect, it } from 'vitest';
import { get } from 'svelte/store';
import { setLocale, t } from '../../../../shared/stores/i18n';
import { errorText, reasonKey, reasonText } from './reason';

describe('reason', () => {
  it('reasonKey per kind', () => {
    expect(reasonKey('knowledge', 'unsupported_type')).toBe('knowledge.reason.unsupported_type');
    expect(reasonKey('api_spec', 'INVALID_SPEC')).toBe('apispec.reason.INVALID_SPEC');
  });
  it('reasonText translates, then falls back to server text, then code', () => {
    setLocale('id');
    const tr = get(t);
    expect(reasonText(tr, 'knowledge', { reason_code: 'unsupported_type', reason: '' })).toBe('Hanya file .md, .markdown, dan .txt yang didukung');
    expect(reasonText(tr, 'knowledge', { reason_code: 'zzz', reason: 'teks server' })).toBe('teks server');
    expect(reasonText(tr, 'knowledge', { reason_code: 'zzz', reason: '' })).toBe('zzz');
  });
  it('too_large uses the unknown-limit wording', () => {
    setLocale('id');
    expect(reasonText(get(t), 'knowledge', { reason_code: 'too_large', reason: '' })).toBe('Ukuran file melebihi batas batas');
  });
  it('errorText', () => {
    setLocale('id');
    expect(errorText(get(t), 'SYNC_IN_PROGRESS', 'x')).toContain('sedang berjalan');
    expect(errorText(get(t), 'UNKNOWN_CODE', 'pesan')).toBe('pesan');
  });
});

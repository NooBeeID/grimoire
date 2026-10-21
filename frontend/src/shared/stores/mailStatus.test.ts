import { get } from 'svelte/store';
import { describe, expect, it } from 'vitest';
import { mailStatus, resetMailStatus, setMailStatus } from './mailStatus';

describe('mailStatus', () => {
  it('mulai null, bisa diset dan direset', () => {
    resetMailStatus();
    expect(get(mailStatus)).toBeNull();
    setMailStatus(false);
    expect(get(mailStatus)).toBe(false);
    setMailStatus(true);
    expect(get(mailStatus)).toBe(true);
    resetMailStatus();
    expect(get(mailStatus)).toBeNull();
  });
});

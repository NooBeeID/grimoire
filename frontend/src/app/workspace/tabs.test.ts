import { describe, expect, it } from 'vitest';
import { defaultTabKey, findTab, workspaceTabs } from './tabs';

describe('workspace tabs', () => {
  it('default is members (before T05)', () => {
    expect(defaultTabKey()).toBe('members');
  });
  it('findTab unknown -> null, known -> tab', () => {
    expect(findTab('xyz')).toBeNull();
    expect(findTab('settings')?.labelKey).toBe('project.tab.settings');
  });
  it('keys are unique', () => {
    const keys = workspaceTabs.map((t) => t.key);
    expect(new Set(keys).size).toBe(keys.length);
  });
});

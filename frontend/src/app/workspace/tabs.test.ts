import { describe, expect, it } from 'vitest';
import { defaultTabKey, findTab, workspaceTabs } from './tabs';

describe('workspace tabs', () => {
  it('order is knowledge, api-keys, members, settings; default is knowledge', () => {
    expect(workspaceTabs.map((t) => t.key)).toEqual(['knowledge', 'api-keys', 'members', 'settings']);
    expect(defaultTabKey()).toBe(workspaceTabs[0]!.key);
    expect(findTab('api-keys')?.labelKey).toBe('project.tab.apiKeys');
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

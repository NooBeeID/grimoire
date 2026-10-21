import type { Component } from 'svelte';
import type { ProjectContext } from '../../domain/project/management/getproject';
import ApiKeysTab from '../views/workspace/ApiKeysTab.svelte';
import MembersTab from '../views/workspace/MembersTab.svelte';
import SettingsTab from '../views/workspace/SettingsTab.svelte';

export interface WorkspaceTabProps {
  ctx: ProjectContext;
  currentUserId: string;
  onprojectgone(): void;
  onleft(): void;
}
export interface WorkspaceTab {
  key: string;
  labelKey: string;
  view: Component<WorkspaceTabProps>;
}

// Registri tab workspace project (rule 12.4: komposisi lintas module hanya di app/).
// Urutan akhir: Knowledge, API Specs, API Keys, Members, Settings (tiket T03, bagian UI).
// T05/T06 cukup menyisipkan satu entri di depan Members tanpa mengubah feature T03:
//   { key: 'knowledge', ... }, { key: 'api-specs', ... }
export const workspaceTabs: WorkspaceTab[] = [
  { key: 'api-keys', labelKey: 'project.tab.apiKeys', view: ApiKeysTab },
  { key: 'members', labelKey: 'project.tab.members', view: MembersTab },
  { key: 'settings', labelKey: 'project.tab.settings', view: SettingsTab },
];

// Tab default = entri pertama.
export function defaultTabKey(tabs: WorkspaceTab[] = workspaceTabs): string {
  return tabs[0]!.key;
}
export function findTab(key: string, tabs: WorkspaceTab[] = workspaceTabs): WorkspaceTab | null {
  return tabs.find((t) => t.key === key) ?? null;
}

import type { Component } from 'svelte';
import type { ProjectContext } from '../../domain/project/management/getproject';
import KnowledgeTab from '../views/workspace/knowledge/KnowledgeTab.svelte';
import ApiSpecsTab from '../views/workspace/ApiSpecsTab.svelte';
import SearchTab from '../views/workspace/SearchTab.svelte';
import ApiKeysTab from '../views/workspace/ApiKeysTab.svelte';
import MembersTab from '../views/workspace/MembersTab.svelte';
import SettingsTab from '../views/workspace/SettingsTab.svelte';

export interface WorkspaceTabProps {
  ctx: ProjectContext;
  currentUserId: string;
  onprojectgone(): void;
  onleft(): void;
  // Segmen ketiga route (/projects/:id/:tab/:item) dan query string; tab tanpa sub-halaman mengabaikannya.
  item?: string | null;
  query?: URLSearchParams;
}
export interface WorkspaceTab {
  key: string;
  labelKey: string;
  view: Component<WorkspaceTabProps>;
  // true bila tab punya sub-halaman (/projects/:id/:tab/:item).
  supportsItem?: boolean;
}

// Registri tab workspace project (rule 12.4: komposisi lintas module hanya di app/).
// Urutan akhir: Knowledge, API Specs, Pencarian (T20), API Keys, Members, Settings (tiket T03, bagian UI).
export const workspaceTabs: WorkspaceTab[] = [
  { key: 'knowledge', labelKey: 'project.tab.knowledge', view: KnowledgeTab },
  { key: 'api-specs', labelKey: 'project.tab.apiSpecs', view: ApiSpecsTab, supportsItem: true },
  { key: 'search', labelKey: 'project.tab.search', view: SearchTab },
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

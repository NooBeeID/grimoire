<script lang="ts">
  import { t } from '../../../shared/stores/i18n';
  import type { WorkspaceTab } from '../../workspace/tabs';

  let { projectId, tabs, active }: { projectId: string; tabs: WorkspaceTab[]; active: string } = $props();
</script>

<!-- Link (bukan role="tablist") karena tiap tab adalah URL sendiri. -->
<nav aria-label={$t('project.tab.label')} class="overflow-x-auto py-1">
  <ul class="glass inline-flex gap-1 rounded-full p-1 whitespace-nowrap shadow-xs">
    {#each tabs as tab (tab.key)}
      <li>
        <a
          href="/projects/{encodeURIComponent(projectId)}/{tab.key}"
          aria-current={tab.key === active ? 'page' : undefined}
          class="inline-flex h-9 items-center rounded-full px-4 text-sm font-medium {tab.key === active
            ? 'bg-primary text-primary-fg shadow-button'
            : 'text-muted hover:bg-surface/70 hover:text-fg'}"
        >
          {$t(tab.labelKey)}
        </a>
      </li>
    {/each}
  </ul>
</nav>

<script lang="ts">
  import { t } from '../../../shared/stores/i18n';
  import type { WorkspaceTab } from '../../workspace/tabs';

  let { projectId, tabs, active }: { projectId: string; tabs: WorkspaceTab[]; active: string } = $props();
</script>

<!-- Link (bukan role="tablist") karena tiap tab adalah URL sendiri. -->
<nav aria-label={$t('project.tab.label')} class="overflow-x-auto border-b border-border">
  <ul class="flex gap-4 whitespace-nowrap">
    {#each tabs as tab (tab.key)}
      <li>
        <a
          href="/projects/{encodeURIComponent(projectId)}/{tab.key}"
          aria-current={tab.key === active ? 'page' : undefined}
          class="inline-block border-b-2 px-1 py-2 hover:underline focus-visible:ring-2 focus-visible:ring-primary focus-visible:outline-none {tab.key === active
            ? 'border-primary font-semibold'
            : 'border-transparent'}"
        >
          {$t(tab.labelKey)}
        </a>
      </li>
    {/each}
  </ul>
</nav>

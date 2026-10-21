<script lang="ts">
  import { t } from '../../../../../shared/stores/i18n';
  import type { ProjectDetail } from '../types';

  let { project }: { project: ProjectDetail } = $props();

  const badge = $derived(
    project.my_role === 'owner'
      ? $t('project.role.owner')
      : project.my_role === 'member'
        ? $t('project.role.member')
        : project.access_level === 'admin' && !project.is_member
          ? $t('project.badge.notMember')
          : null,
  );
</script>

<header class="flex flex-col gap-2">
  <div class="flex flex-wrap items-center gap-3">
    <h1 class="text-2xl font-bold break-words">{project.name}</h1>
    {#if badge}
      <span class="inline-flex items-center rounded-base border border-border px-2 py-0.5 text-xs font-medium text-muted">{badge}</span>
    {/if}
  </div>
  {#if project.description}<p class="break-words whitespace-pre-line">{project.description}</p>{/if}
  <p class="text-sm text-muted">{$t('project.detail.memberCount', { n: project.member_count })}</p>
</header>

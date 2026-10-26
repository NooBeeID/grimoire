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

<header class="flex flex-col gap-1">
  <div class="flex flex-wrap items-center gap-3">
    <h1 class="text-2xl leading-8 font-semibold break-words">{project.name}</h1>
    {#if badge}
      <span class="inline-flex h-5 items-center rounded-full bg-active px-2 text-xs font-medium text-fg">{badge}</span>
    {/if}
  </div>
  {#if project.description}<p class="text-sm break-words whitespace-pre-line text-muted">{project.description}</p>{/if}
  <p class="text-sm text-muted">{$t('project.detail.memberCount', { n: project.member_count })}</p>
</header>

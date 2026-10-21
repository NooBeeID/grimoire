<script lang="ts">
  import { locale, t } from '../../../../../shared/stores/i18n';
  import type { ProjectSummary } from '../types';
  import { excerpt, formatCount, formatRelative } from './format';
  import RoleBadge from './RoleBadge.svelte';

  let { items, onopen }: { items: ProjectSummary[]; onopen?: (id: string) => void } = $props();

  const cols = 'md:grid md:grid-cols-[minmax(0,1fr)_8rem_4rem_4rem_9rem] md:items-center md:gap-4';
  // Klik biasa ditangani router global; onopen hanya untuk konsumen tanpa router.
  function click(e: MouseEvent, id: string) {
    if (!onopen || e.defaultPrevented || e.button !== 0 || e.ctrlKey || e.metaKey || e.shiftKey || e.altKey) return;
    e.preventDefault();
    onopen(id);
  }
</script>

<div class="flex flex-col">
  <div class="hidden border-b border-border px-3 py-2 text-sm font-semibold md:grid md:grid-cols-[minmax(0,1fr)_8rem_4rem_4rem_9rem] md:gap-4" aria-hidden="true">
    <span>{$t('project.list.col.name')}</span>
    <span>{$t('project.list.col.role')}</span>
    <span>{$t('project.list.col.files')}</span>
    <span>{$t('project.list.col.specs')}</span>
    <span>{$t('project.list.col.updated')}</span>
  </div>
  <ul class="flex flex-col gap-2 md:gap-0">
    {#each items as p (p.id)}
      <li
        data-project-id={p.id}
        class="flex flex-col gap-2 rounded-base border border-border p-3 md:rounded-none md:border-0 md:border-b {cols}"
      >
        <div class="min-w-0">
          <a href="/projects/{encodeURIComponent(p.id)}" onclick={(e) => click(e, p.id)} class="font-medium break-words hover:underline focus-visible:ring-2 focus-visible:ring-primary focus-visible:outline-none">{p.name}</a>
          {#if p.description}<p class="text-sm break-words text-muted">{excerpt(p.description)}</p>{/if}
        </div>
        <div>
          <RoleBadge myRole={p.my_role} isMember={p.is_member} accessLevel={p.access_level} />
        </div>
        <div class="text-sm">
          <span class="text-muted md:sr-only">{$t('project.list.col.files')}: </span>{formatCount(p.counts.files, $locale)}
        </div>
        <div class="text-sm">
          <span class="text-muted md:sr-only">{$t('project.list.col.specs')}: </span>{formatCount(p.counts.specs, $locale)}
        </div>
        <div class="text-sm text-muted">
          <time datetime={p.updated_at} title={p.updated_at}>{formatRelative(p.updated_at, $locale)}</time>
        </div>
      </li>
    {/each}
  </ul>
</div>

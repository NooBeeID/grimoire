<script lang="ts">
  import Avatar from '../../../../../shared/components/Avatar.svelte';
  import { locale, t } from '../../../../../shared/stores/i18n';
  import type { ProjectSummary } from '../types';
  import { excerpt, formatCount, formatRelative } from './format';
  import RoleBadge from './RoleBadge.svelte';

  let { items, onopen }: { items: ProjectSummary[]; onopen?: (id: string) => void } = $props();

  // Kolom hitungan dibuang bila belum ada satu pun baris yang punya nilainya (tidak ada kolom yang berisi "-" semua).
  const showFiles = $derived(items.some((p) => p.counts.files !== null));
  const showSpecs = $derived(items.some((p) => p.counts.specs !== null));
  const cols = $derived(
    ['minmax(0,1fr)', '5.5rem', '8rem', showFiles ? '4rem' : '', showSpecs ? '4rem' : '', '9rem'].filter(Boolean).join(' '),
  );
  // Klik biasa ditangani router global; onopen hanya untuk konsumen tanpa router.
  function click(e: MouseEvent, id: string) {
    if (!onopen || e.defaultPrevented || e.button !== 0 || e.ctrlKey || e.metaKey || e.shiftKey || e.altKey) return;
    e.preventDefault();
    onopen(id);
  }
</script>

<div class="overflow-hidden rounded-card border border-border bg-surface shadow-panel" style="--cols: {cols}">
  <div class="hidden h-10 items-center gap-4 bg-raised px-4 text-[13px] font-medium text-muted md:grid md:grid-cols-[var(--cols)]" aria-hidden="true">
    <span>{$t('project.list.col.name')}</span>
    <span class="text-right">{$t('project.list.col.members')}</span>
    <span>{$t('project.list.col.role')}</span>
    {#if showFiles}<span class="text-right">{$t('project.list.col.files')}</span>{/if}
    {#if showSpecs}<span class="text-right">{$t('project.list.col.specs')}</span>{/if}
    <span class="text-right">{$t('project.list.col.updated')}</span>
  </div>
  <ul class="flex flex-col [&>li:first-child]:border-t-0 md:[&>li:first-child]:border-t">
    {#each items as p (p.id)}
      <li
        data-project-id={p.id}
        class="flex flex-wrap items-center gap-x-3 gap-y-1 border-t border-rule px-4 py-3 transition-colors hover:bg-raised/60 md:grid md:min-h-14 md:grid-cols-[var(--cols)] md:items-center md:gap-4"
      >
        <div class="flex w-full min-w-0 items-center gap-3 md:w-auto">
          <Avatar name={p.name} size={34} shape="square" />
          <div class="min-w-0">
            <a href="/projects/{encodeURIComponent(p.id)}" onclick={(e) => click(e, p.id)} class="font-medium break-words hover:underline focus-visible:ring-2 focus-visible:ring-accent-ink focus-visible:outline-none">{p.name}</a>
            {#if p.description}<p class="text-[13px] break-words text-muted">{excerpt(p.description)}</p>{/if}
          </div>
        </div>
        <div class="text-sm md:text-right">
          <span class="mr-1 text-muted md:sr-only">{$t('project.list.col.members')}:</span>{p.member_count}
        </div>
        <div>
          <RoleBadge myRole={p.my_role} isMember={p.is_member} accessLevel={p.access_level} />
        </div>
        {#if showFiles}
          <div class="text-sm md:text-right">
            <span class="text-muted md:sr-only">{$t('project.list.col.files')}: </span>{formatCount(p.counts.files, $locale)}
          </div>
        {/if}
        {#if showSpecs}
          <div class="text-sm md:text-right">
            <span class="text-muted md:sr-only">{$t('project.list.col.specs')}: </span>{formatCount(p.counts.specs, $locale)}
          </div>
        {/if}
        <div class="text-sm text-muted md:text-right">
          <time datetime={p.updated_at} title={p.updated_at}>{formatRelative(p.updated_at, $locale)}</time>
        </div>
      </li>
    {/each}
  </ul>
</div>

<script lang="ts">
  import MarkdownView from '../../../../../shared/components/MarkdownView.svelte';
  import { t } from '../../../../../shared/stores/i18n';
  import { endpointKey, UNTAGGED, type TagGroup } from '../model/operations';
  import type { ApiSpecEndpoint } from '../types';
  import MethodBadge from './MethodBadge.svelte';

  let {
    tree,
    matches,
    query,
    selected,
    total,
    hrefFor,
    onquery,
  }: {
    tree: TagGroup[];
    matches: ApiSpecEndpoint[];
    query: string;
    selected: string | null;
    total: number;
    hrefFor: (ep: ApiSpecEndpoint) => string;
    onquery: (q: string) => void;
  } = $props();

  const searching = $derived(query.trim() !== '');
</script>

{#snippet item(ep: ApiSpecEndpoint)}
  {@const key = endpointKey(ep.method, ep.path)}
  <li>
    <a
      href={hrefFor(ep)}
      aria-current={selected === key ? 'page' : undefined}
      class="flex items-start gap-2.5 rounded-base px-2 py-1.5 text-sm hover:bg-raised focus-visible:ring-2 focus-visible:ring-accent-ink focus-visible:outline-none {selected === key ? 'bg-active' : ''}"
    >
      <span class="pt-px"><MethodBadge method={ep.method} /></span>
      <span class="min-w-0">
        <span class="block break-all font-mono text-[13px] {selected === key ? 'font-medium' : ''} {ep.deprecated ? 'line-through' : ''}">{ep.path}</span>
        {#if ep.summary}<span class="block text-xs text-muted">{ep.summary}</span>{/if}
        {#if ep.deprecated}<span class="text-xs text-warning">{$t('apispec.doc.deprecated')}</span>{/if}
      </span>
    </a>
  </li>
{/snippet}

<div class="flex flex-col overflow-hidden outlined rounded-card bg-surface shadow-panel md:max-h-[calc(100vh-9rem)]">
  <div class="flex items-center gap-2 bg-raised px-3 py-2">
    <input
      type="search"
      value={query}
      placeholder={$t('apispec.detail.search')}
      aria-label={$t('apispec.detail.search')}
      oninput={(e) => onquery(e.currentTarget.value)}
      class="h-8 min-w-0 flex-1 rounded-base border border-control bg-surface px-3 text-sm focus-visible:ring-2 focus-visible:ring-accent-ink focus-visible:outline-none"
    />
    <span class="shrink-0 text-xs text-muted tabular-nums">{total}</span>
  </div>
  <nav aria-label={$t('apispec.detail.endpoints')} class="flex min-h-0 flex-col gap-1 overflow-auto p-2">
    {#if total === 0}
      <p class="px-2 py-1 text-sm text-muted">{$t('apispec.detail.noEndpoints')}</p>
    {:else if searching}
      <p class="px-2 text-xs text-muted" aria-live="polite">{$t('apispec.detail.searchCount', { n: matches.length })}</p>
      {#if matches.length === 0}
        <p class="px-2 py-1 text-sm text-muted">{$t('apispec.detail.searchEmpty')}</p>
      {:else}
        <ul class="flex flex-col gap-0.5">{#each matches as ep (endpointKey(ep.method, ep.path))}{@render item(ep)}{/each}</ul>
      {/if}
    {:else}
      {#each tree as g (g.name)}
        <details open class="group">
          <summary class="flex cursor-pointer list-none items-center gap-1.5 rounded-base px-2 py-1.5 text-[13px] font-semibold hover:bg-raised focus-visible:ring-2 focus-visible:ring-accent-ink focus-visible:outline-none [&::-webkit-details-marker]:hidden">
            <svg aria-hidden="true" viewBox="0 0 16 16" class="size-3 text-muted transition-transform group-open:rotate-90" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M6 3.5 10.5 8 6 12.5" /></svg>
            <span class="min-w-0 flex-1">{g.name === UNTAGGED ? $t('apispec.detail.untagged') : g.name}</span>
            <span class="font-normal text-muted tabular-nums">{g.endpoints.length}</span>
          </summary>
          {#if g.description}<div class="px-2 pb-1 text-xs text-muted"><MarkdownView source={g.description} /></div>{/if}
          <ul class="flex flex-col gap-0.5">{#each g.endpoints as ep (endpointKey(ep.method, ep.path))}{@render item(ep)}{/each}</ul>
        </details>
      {/each}
    {/if}
  </nav>
</div>

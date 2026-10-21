<script lang="ts">
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
      class="flex items-start gap-2 rounded-base px-2 py-1.5 text-sm hover:bg-raised focus-visible:ring-2 focus-visible:ring-accent-ink focus-visible:outline-none {selected === key ? 'bg-active' : ''}"
    >
      <MethodBadge method={ep.method} />
      <span class="min-w-0">
        <span class="block break-all font-mono text-[13px] {ep.deprecated ? 'line-through' : ''}">{ep.path}</span>
        {#if ep.summary}<span class="block text-xs text-muted">{ep.summary}</span>{/if}
        {#if ep.deprecated}<span class="text-xs text-warning">{$t('apispec.doc.deprecated')}</span>{/if}
      </span>
    </a>
  </li>
{/snippet}

<div class="flex flex-col gap-3">
  <input
    type="search"
    value={query}
    placeholder={$t('apispec.detail.search')}
    aria-label={$t('apispec.detail.search')}
    oninput={(e) => onquery(e.currentTarget.value)}
    class="h-8 rounded-base border border-control bg-surface px-3 text-sm focus-visible:ring-2 focus-visible:ring-accent-ink focus-visible:outline-none"
  />
  <nav aria-label={$t('apispec.detail.endpoints')} class="flex flex-col gap-2">
    {#if total === 0}
      <p class="text-sm text-muted">{$t('apispec.detail.noEndpoints')}</p>
    {:else if searching}
      <p class="text-xs text-muted" aria-live="polite">{$t('apispec.detail.searchCount', { n: matches.length })}</p>
      {#if matches.length === 0}
        <p class="text-sm text-muted">{$t('apispec.detail.searchEmpty')}</p>
      {:else}
        <ul class="flex flex-col gap-0.5">{#each matches as ep (endpointKey(ep.method, ep.path))}{@render item(ep)}{/each}</ul>
      {/if}
    {:else}
      {#each tree as g (g.name)}
        <details open>
          <summary class="cursor-pointer py-1 text-[13px] font-semibold">
            {g.name === UNTAGGED ? $t('apispec.detail.untagged') : g.name}
            <span class="font-normal text-muted tabular-nums">{g.endpoints.length}</span>
          </summary>
          <ul class="flex flex-col gap-0.5">{#each g.endpoints as ep (endpointKey(ep.method, ep.path))}{@render item(ep)}{/each}</ul>
        </details>
      {/each}
    {/if}
  </nav>
</div>

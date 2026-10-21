<script lang="ts">
  import Table from '../../../../../shared/components/Table.svelte';
  import type { ApiError } from '../../../../../shared/services/http';
  import { locale, t } from '../../../../../shared/stores/i18n';
  import { formatDateTime } from '../../../../../shared/utils/datetime';
  import type { ApiSpec } from '../types';
  import OpenApiBadge from './OpenApiBadge.svelte';

  let {
    items,
    projectId,
    loading = false,
    error = null,
    onretry,
  }: {
    items: ApiSpec[];
    projectId: string;
    loading?: boolean;
    error?: ApiError | null;
    onretry?: () => void;
  } = $props();

  const columns = $derived([
    { key: 'name', header: $t('apispec.list.col.name') },
    { key: 'version', header: $t('apispec.list.col.version') },
    { key: 'endpoints', header: $t('apispec.list.col.endpoints'), class: 'text-right' },
    { key: 'source', header: $t('apispec.list.col.source') },
    { key: 'updated', header: $t('apispec.list.col.updated') },
  ]);
</script>

{#snippet cell(s: ApiSpec, col: { key: string })}
  {#if col.key === 'name'}
    <a
      href="/projects/{encodeURIComponent(projectId)}/api-specs/{encodeURIComponent(s.name)}"
      class="font-mono text-[13px] text-fg underline-offset-2 hover:underline focus-visible:ring-2 focus-visible:ring-accent-ink focus-visible:outline-none">{s.name}</a
    >
    <span class="block break-words text-xs text-muted">{s.title}</span>
  {:else if col.key === 'version'}
    <span class="inline-flex items-center gap-2"><OpenApiBadge version={s.openapi_version} /><span>{s.api_version}</span></span>
  {:else if col.key === 'endpoints'}
    <span class="tabular-nums">{s.endpoint_count}</span>
  {:else if col.key === 'source'}
    {#if s.source_path}
      <code class="block max-w-56 truncate font-mono text-[13px]" title={s.source_path}>{s.source_path}</code>
    {:else}
      <span class="text-muted">–</span>
    {/if}
  {:else}
    <time datetime={s.updated_at}>{formatDateTime(s.updated_at, $locale)}</time>
    {#if s.updated_by_label}<span class="block text-xs text-muted">{s.updated_by_label}</span>{/if}
  {/if}
{/snippet}

<div class="overflow-x-auto">
  <Table
    {columns}
    rows={items}
    rowKey={(s) => s.name}
    {cell}
    {loading}
    {error}
    {onretry}
    caption={$t('apispec.title')}
    rowAttrs={(s) => ({ 'data-spec': s.name })}
  />
</div>

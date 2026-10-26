<script lang="ts">
  import Button from '../../../../../shared/components/Button.svelte';
  import Table from '../../../../../shared/components/Table.svelte';
  import type { ApiError } from '../../../../../shared/services/http';
  import { locale, t } from '../../../../../shared/stores/i18n';
  import { formatDateTime } from '../../../../../shared/utils/datetime';
  import type { SynonymGroup } from '../types';

  let {
    items,
    loading = false,
    error = null,
    onretry,
    onedit,
    ondelete,
  }: {
    items: SynonymGroup[];
    loading?: boolean;
    error?: ApiError | null;
    onretry?: () => void;
    onedit: (g: SynonymGroup) => void;
    ondelete: (g: SynonymGroup) => void;
  } = $props();

  const columns = $derived([
    { key: 'terms', header: $t('searchSynonym.col.terms') },
    { key: 'updatedBy', header: $t('searchSynonym.col.updatedBy') },
    { key: 'updated', header: $t('searchSynonym.col.updated') },
    { key: 'actions', header: $t('searchSynonym.col.actions'), class: 'text-right' },
  ]);
  const label = (g: SynonymGroup) => g.terms.join(', ');
</script>

{#snippet cell(g: SynonymGroup, col: { key: string })}
  {#if col.key === 'terms'}
    <ul class="flex flex-wrap gap-1.5" aria-label={label(g)}>
      {#each g.terms as term (term)}
        <li class="outlined rounded-full bg-raised px-2.5 py-0.5 text-[13px]">{term}</li>
      {/each}
    </ul>
  {:else if col.key === 'updatedBy'}
    <span class="break-words">{g.updated_by_label}</span>
  {:else if col.key === 'updated'}
    <time datetime={g.updated_at}>{formatDateTime(g.updated_at, $locale)}</time>
  {:else}
    <div class="flex items-center justify-end gap-1">
      <Button size="sm" variant="ghost" ariaLabel={$t('searchSynonym.edit.aria', { terms: label(g) })} onclick={() => onedit(g)}>
        {$t('searchSynonym.edit.open')}
      </Button>
      <Button size="sm" variant="ghost" ariaLabel={$t('searchSynonym.delete.aria', { terms: label(g) })} onclick={() => ondelete(g)}>
        {$t('searchSynonym.delete.open')}
      </Button>
    </div>
  {/if}
{/snippet}

<div class="overflow-x-auto">
  <Table
    {columns}
    rows={items}
    rowKey={(g) => g.id}
    {cell}
    {loading}
    {error}
    {onretry}
    caption={$t('searchSynonym.title')}
    rowAttrs={(g) => ({ 'data-group-id': g.id })}
  />
</div>

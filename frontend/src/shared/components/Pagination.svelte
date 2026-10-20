<script lang="ts">
  import { t } from '../stores/i18n';
  import Button from './Button.svelte';

  let {
    page,
    pageSize,
    total,
    onchange,
  }: { page: number; pageSize: number; total: number; onchange: (page: number) => void } = $props();

  const pages = $derived(Math.max(1, Math.ceil(total / pageSize)));
</script>

<nav aria-label={$t('pagination.label')} class="flex items-center justify-between gap-2">
  <Button variant="secondary" size="sm" disabled={page <= 1} onclick={() => onchange(page - 1)}>
    {$t('pagination.prev')}
  </Button>
  <span aria-current="page" class="text-sm">{$t('pagination.page_of', { page, pages })}</span>
  <Button variant="secondary" size="sm" disabled={page >= pages} onclick={() => onchange(page + 1)}>
    {$t('pagination.next')}
  </Button>
</nav>

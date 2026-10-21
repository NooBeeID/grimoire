<script lang="ts">
  import { t } from '../../../../../shared/stores/i18n';

  let { total, limit }: { total: number; limit: number } = $props();
</script>

<span class="text-sm font-normal text-muted">{$t('apispec.list.quota', { total, limit })}</span>
{#if total >= limit}
  <p role="status" class="mt-1 text-sm text-warning">{$t('apispec.list.limitReached')}</p>
{/if}

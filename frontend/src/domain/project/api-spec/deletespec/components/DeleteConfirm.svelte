<script lang="ts">
  import { t } from '../../../../../shared/stores/i18n';

  let { name, error, traceId }: { name: string; error?: string | null; traceId?: string } = $props();
</script>

<div class="flex flex-col gap-3">
  <p class="text-sm">{$t('apispec.delete.message', { name })}</p>
  <p class="text-sm text-muted">{$t('apispec.delete.effect')}</p>
  {#if error}
    <p role="alert" class="text-sm text-danger">{error}</p>
    {#if traceId}<p class="text-xs text-muted">{$t('error.trace_id', { id: traceId })}</p>{/if}
  {/if}
</div>

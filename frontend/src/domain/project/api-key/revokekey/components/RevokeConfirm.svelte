<script lang="ts">
  import { t } from '../../../../../shared/stores/i18n';
  import type { RevokeTarget } from '../types';

  let { target, error = null, traceId }: { target: RevokeTarget; error?: string | null; traceId?: string } = $props();
  const parts = $derived($t('project.apikey.revoke.message', { name: target.name, prefix: '\u0000' }).split('\u0000'));
</script>

<div class="flex flex-col gap-3 text-sm">
  <p class="break-words">{parts[0]}<code class="font-mono text-[13px]">{target.prefix}</code>{parts[1] ?? ''}</p>
  <p class="text-muted">{$t('project.apikey.revoke.effect')}</p>
  <div role="alert">
    {#if error}
      <p class="text-danger">{error}</p>
      {#if traceId}<p class="text-xs text-muted">{$t('error.trace_id', { id: traceId })}</p>{/if}
    {/if}
  </div>
</div>

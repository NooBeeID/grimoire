<script lang="ts">
  import Button from '../../../../../shared/components/Button.svelte';
  import Dialog from '../../../../../shared/components/Dialog.svelte';
  import { isApiError } from '../../../../../shared/services/http';
  import { t } from '../../../../../shared/stores/i18n';
  import { toast } from '../../../../../shared/stores/toast';
  import { deleteCredential } from '../services/deletecredential.service';
  import type { DeleteTarget } from '../types';

  let {
    open,
    target,
    onclose,
    ondeleted,
  }: {
    open: boolean;
    target: DeleteTarget | null;
    onclose: () => void;
    ondeleted: () => void;
  } = $props();

  let submitting = $state(false);
  let error = $state<string | null>(null);
  let traceId = $state<string | undefined>();

  $effect(() => {
    if (open) {
      error = null;
      traceId = undefined;
    }
  });

  async function confirm() {
    if (!target || submitting) return;
    const tgt = target;
    const name = $t(`gitCredential.provider.${tgt.provider}`);
    submitting = true;
    error = null;
    try {
      await deleteCredential(tgt);
      onclose();
      ondeleted();
      toast.push({ type: 'success', message: $t('gitCredential.delete.success', { provider: name }) });
    } catch (e) {
      if (isApiError(e) && e.status === 404) {
        onclose();
        ondeleted();
        toast.push({ type: 'info', message: $t('gitCredential.delete.gone') });
      } else if (isApiError(e)) {
        error = e.message;
        traceId = e.traceId;
      } else {
        error = $t('error.internal');
      }
    } finally {
      submitting = false;
    }
  }
</script>

<Dialog {open} title={$t('gitCredential.delete.title')} dismissible={!submitting} {onclose}>
  {#if target}
    <div class="flex flex-col gap-3">
      <p class="text-sm">{$t('gitCredential.delete.confirm', { provider: $t(`gitCredential.provider.${target.provider}`) })}</p>
      <p class="flex flex-wrap items-center gap-2 rounded-base bg-raised px-3 py-2 text-[13px]">
        <span>{$t(`gitCredential.provider.${target.provider}`)}</span>
        <code class="font-mono">{target.host}</code>
        <code class="font-mono text-muted">…{target.last4}</code>
      </p>
      {#if error}
        <p role="alert" class="text-sm text-danger">
          {error}
          {#if traceId}<span class="block text-xs text-muted">{$t('error.trace_id', { id: traceId })}</span>{/if}
        </p>
      {/if}
    </div>
  {/if}
  {#snippet footer()}
    <Button variant="secondary" disabled={submitting} onclick={onclose}>{$t('common.cancel')}</Button>
    <Button variant="danger" loading={submitting} onclick={confirm}>{$t('gitCredential.delete.submit')}</Button>
  {/snippet}
</Dialog>

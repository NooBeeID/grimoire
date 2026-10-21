<script lang="ts">
  import Button from '../../../../../shared/components/Button.svelte';
  import Dialog from '../../../../../shared/components/Dialog.svelte';
  import { isApiError } from '../../../../../shared/services/http';
  import { t } from '../../../../../shared/stores/i18n';
  import { toast } from '../../../../../shared/stores/toast';
  import DeleteConfirm from '../components/DeleteConfirm.svelte';
  import { deleteSpec } from '../services/deletespec.service';

  let {
    open,
    projectId,
    spec,
    onclose,
    ondeleted,
  }: {
    open: boolean;
    projectId: string;
    spec: { name: string } | null;
    onclose: () => void;
    ondeleted: (name: string) => void;
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
    if (!spec || submitting) return;
    const name = spec.name;
    submitting = true;
    error = null;
    try {
      await deleteSpec(projectId, name);
      onclose();
      ondeleted(name);
      toast.push({ type: 'success', message: $t('apispec.delete.success', { name }) });
    } catch (e) {
      if (isApiError(e) && e.status === 404) {
        // Sudah tidak ada: anggap terhapus.
        onclose();
        ondeleted(name);
        toast.push({ type: 'error', message: $t('apispec.delete.alreadyGone', { name }) });
      } else if (isApiError(e)) {
        error = e.message;
        traceId = e.traceId;
      } else error = String(e);
    } finally {
      submitting = false;
    }
  }
</script>

<Dialog {open} title={$t('apispec.delete.title')} dismissible={!submitting} {onclose}>
  {#if spec}<DeleteConfirm name={spec.name} {error} {traceId} />{/if}
  {#snippet footer()}
    <Button variant="secondary" disabled={submitting} onclick={onclose}>{$t('common.cancel')}</Button>
    <Button variant="danger" loading={submitting} onclick={confirm}>{$t('apispec.delete.submit')}</Button>
  {/snippet}
</Dialog>

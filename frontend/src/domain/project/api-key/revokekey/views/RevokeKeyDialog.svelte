<script lang="ts">
  import Button from '../../../../../shared/components/Button.svelte';
  import Dialog from '../../../../../shared/components/Dialog.svelte';
  import { isApiError } from '../../../../../shared/services/http';
  import { t } from '../../../../../shared/stores/i18n';
  import { toast } from '../../../../../shared/stores/toast';
  import { reasonOf } from '../../../../../shared/validation/error';
  import RevokeConfirm from '../components/RevokeConfirm.svelte';
  import { revokeKey } from '../services/revokekey.service';
  import type { RevokedKey, RevokeTarget } from '../types';

  let {
    open,
    projectId,
    target,
    onclose,
    onrevoked,
    onstale,
    onprojectgone,
  }: {
    open: boolean;
    projectId: string;
    target: RevokeTarget | null;
    onclose: () => void;
    onrevoked: (key: RevokedKey) => void;
    onstale: () => void;
    onprojectgone: () => void;
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
    submitting = true;
    error = null;
    try {
      const key = await revokeKey(projectId, tgt.id);
      onclose();
      onrevoked(key);
      toast.push({ type: 'success', message: $t('project.apikey.revoke.success', { name: tgt.name }) });
    } catch (e) {
      const reason = isApiError(e) ? reasonOf(e.details) : null;
      if (isApiError(e) && e.status === 409 && reason === 'ALREADY_REVOKED') {
        onclose();
        toast.push({ type: 'error', message: $t('project.apikey.error.ALREADY_REVOKED') });
        onstale();
      } else if (isApiError(e) && e.status === 404 && reason === 'KEY_NOT_FOUND') {
        onclose();
        toast.push({ type: 'error', message: $t('project.apikey.error.KEY_NOT_FOUND') });
        onstale();
      } else if (isApiError(e) && e.status === 404) {
        onclose();
        onprojectgone();
      } else if (isApiError(e)) {
        error = e.message;
        traceId = e.traceId;
      } else {
        error = String(e);
      }
    } finally {
      submitting = false;
    }
  }
</script>

<Dialog {open} title={$t('project.apikey.revoke.title')} dismissible={!submitting} {onclose}>
  {#if target}<RevokeConfirm {target} {error} {traceId} />{/if}
  {#snippet footer()}
    <Button variant="secondary" disabled={submitting} onclick={onclose}>{$t('common.cancel')}</Button>
    <Button variant="danger" loading={submitting} onclick={confirm}>{$t('project.apikey.revoke.submit')}</Button>
  {/snippet}
</Dialog>

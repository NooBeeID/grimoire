<script lang="ts">
  import Button from '../../../../../shared/components/Button.svelte';
  import Dialog from '../../../../../shared/components/Dialog.svelte';
  import { isApiError } from '../../../../../shared/services/http';
  import { t } from '../../../../../shared/stores/i18n';
  import { toast } from '../../../../../shared/stores/toast';
  import type { SynonymGroup } from '../../listgroups';
  import { deleteGroup } from '../services/deletegroup.service';

  let {
    open,
    projectId,
    group,
    onclose,
    ondeleted,
    onstale,
    onprojectgone,
  }: {
    open: boolean;
    projectId: string;
    group: SynonymGroup | null;
    onclose: () => void;
    ondeleted: (id: string) => void;
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
    if (!group || submitting) return;
    const g = group;
    submitting = true;
    error = null;
    try {
      await deleteGroup(projectId, g.id);
      onclose();
      ondeleted(g.id);
      toast.push({ type: 'success', message: $t('searchSynonym.deleted') });
    } catch (e) {
      if (isApiError(e) && e.status === 404) {
        onclose();
        toast.push({ type: 'error', message: $t('searchSynonym.gone') });
        onstale();
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

<Dialog {open} title={$t('searchSynonym.delete.title')} dismissible={!submitting} {onclose}>
  {#if group}
    <div class="flex flex-col gap-3 text-sm">
      <p class="break-words">{$t('searchSynonym.delete.message')}</p>
      <ul class="flex flex-wrap gap-1.5">
        {#each group.terms as term (term)}
          <li class="outlined rounded-full bg-raised px-2.5 py-0.5 text-[13px]">{term}</li>
        {/each}
      </ul>
      <div role="alert">
        {#if error}
          <p class="text-danger">{error}</p>
          {#if traceId}<p class="text-xs text-muted">{$t('error.trace_id', { id: traceId })}</p>{/if}
        {/if}
      </div>
    </div>
  {/if}
  {#snippet footer()}
    <Button variant="secondary" disabled={submitting} onclick={onclose}>{$t('common.cancel')}</Button>
    <Button variant="danger" loading={submitting} onclick={confirm}>{$t('searchSynonym.delete.submit')}</Button>
  {/snippet}
</Dialog>

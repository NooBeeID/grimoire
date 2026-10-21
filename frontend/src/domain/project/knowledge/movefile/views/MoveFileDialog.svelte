<script lang="ts">
  import Button from '../../../../../shared/components/Button.svelte';
  import Dialog from '../../../../../shared/components/Dialog.svelte';
  import { isApiError } from '../../../../../shared/services/http';
  import { t } from '../../../../../shared/stores/i18n';
  import { toast } from '../../../../../shared/stores/toast';
  import { errorMessage, reasonMessageKey, validateNewFilePath, type KnowledgeFile } from '../../base';
  import MoveFileForm from '../components/MoveFileForm.svelte';
  import { moveFile } from '../services/movefile.service';

  let {
    open,
    projectId,
    from,
    onmoved,
    onclose,
    onnotfound,
  }: {
    open: boolean;
    projectId: string;
    from: string;
    onmoved: (file: KnowledgeFile) => void;
    onclose: () => void;
    onnotfound: () => void;
  } = $props();

  let to = $state('');
  let submitting = $state(false);
  let serverError = $state<string | null>(null);

  $effect(() => {
    if (open) {
      to = from;
      serverError = null;
    }
  });

  const reason = $derived(to === from ? 'same_path' : validateNewFilePath(to));
  const error = $derived(serverError ?? (reason && to !== from ? $t(reasonMessageKey(reason) ?? 'error.internal') : undefined));

  async function submit(e: Event) {
    e.preventDefault();
    if (reason || submitting) return;
    submitting = true;
    serverError = null;
    try {
      const f = await moveFile(projectId, { from, to });
      toast.push({ type: 'success', message: $t('knowledge.move.done') });
      onmoved(f);
    } catch (err) {
      if (isApiError(err) && err.status === 404) {
        toast.push({ type: 'error', message: $t('knowledge.move.sourceGone') });
        onnotfound();
      } else serverError = errorMessage(err, $t);
    } finally {
      submitting = false;
    }
  }
</script>

<Dialog {open} title={$t('knowledge.move.title')} {onclose}>
  <MoveFileForm {from} bind:to {error} active={open} onsubmit={submit} />
  {#snippet footer()}
    <Button variant="secondary" onclick={onclose}>{$t('knowledge.editor.cancel')}</Button>
    <Button type="submit" loading={submitting} disabled={reason !== null} onclick={submit}>{$t('knowledge.move.save')}</Button>
  {/snippet}
</Dialog>

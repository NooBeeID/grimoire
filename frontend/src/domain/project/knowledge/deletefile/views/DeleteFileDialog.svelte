<script lang="ts">
  import Button from '../../../../../shared/components/Button.svelte';
  import Dialog from '../../../../../shared/components/Dialog.svelte';
  import { isApiError } from '../../../../../shared/services/http';
  import { t } from '../../../../../shared/stores/i18n';
  import { toast } from '../../../../../shared/stores/toast';
  import { errorMessage } from '../../base';
  import { deleteFile } from '../services/deletefile.service';

  let {
    open,
    projectId,
    path,
    ondeleted,
    onclose,
  }: { open: boolean; projectId: string; path: string; ondeleted: (path: string) => void; onclose: () => void } = $props();

  let busy = $state(false);
  let error = $state<string | null>(null);

  $effect(() => {
    if (open) error = null;
  });

  async function confirm() {
    busy = true;
    error = null;
    try {
      await deleteFile(projectId, path);
      toast.push({ type: 'success', message: $t('knowledge.deleteFile.done') });
      ondeleted(path);
    } catch (e) {
      if (isApiError(e) && e.status === 404) {
        toast.push({ type: 'info', message: $t('knowledge.deleteFile.gone') });
        ondeleted(path);
      } else error = errorMessage(e, $t);
    } finally {
      busy = false;
    }
  }
</script>

<Dialog {open} title={$t('knowledge.deleteFile.title')} {onclose}>
  <p class="font-mono text-[13px] break-all">{path}</p>
  <p class="mt-2 text-sm text-muted">{$t('knowledge.deleteFile.body')}</p>
  {#if error}<p role="alert" class="mt-2 text-sm text-danger">{error}</p>{/if}
  {#snippet footer()}
    <Button variant="secondary" onclick={onclose}>{$t('knowledge.editor.cancel')}</Button>
    <Button variant="danger" loading={busy} onclick={confirm}>{$t('knowledge.deleteFile.confirm')}</Button>
  {/snippet}
</Dialog>

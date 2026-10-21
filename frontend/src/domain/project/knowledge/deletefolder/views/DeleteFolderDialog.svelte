<script lang="ts">
  import Button from '../../../../../shared/components/Button.svelte';
  import Dialog from '../../../../../shared/components/Dialog.svelte';
  import { isApiError } from '../../../../../shared/services/http';
  import { t } from '../../../../../shared/stores/i18n';
  import { toast } from '../../../../../shared/stores/toast';
  import { errorMessage } from '../../base';
  import { deleteFolder } from '../services/deletefolder.service';

  let {
    open,
    projectId,
    path,
    fileCount,
    ondeleted,
    onclose,
  }: {
    open: boolean;
    projectId: string;
    path: string;
    fileCount: number;
    ondeleted: (path: string, deleted: number) => void;
    onclose: () => void;
  } = $props();

  let ack = $state(false);
  let busy = $state(false);
  let error = $state<string | null>(null);

  $effect(() => {
    if (open) {
      ack = false;
      error = null;
    }
  });

  async function confirm() {
    busy = true;
    error = null;
    try {
      const r = await deleteFolder(projectId, path);
      toast.push({ type: 'success', message: $t('knowledge.deleteFolder.done', { count: r.deleted }) });
      ondeleted(path, r.deleted);
    } catch (e) {
      if (isApiError(e) && e.status === 404) {
        toast.push({ type: 'info', message: $t('knowledge.deleteFolder.gone') });
        ondeleted(path, 0);
      } else error = errorMessage(e, $t);
    } finally {
      busy = false;
    }
  }
</script>

<Dialog {open} title={$t('knowledge.deleteFolder.title')} {onclose}>
  <p class="font-mono text-[13px] break-all">{path}</p>
  <p class="mt-2 text-sm text-muted">{$t('knowledge.deleteFolder.body', { count: fileCount })}</p>
  <label class="mt-3 flex items-start gap-2 text-sm">
    <input type="checkbox" bind:checked={ack} class="mt-0.5 accent-primary" />
    <span>{$t('knowledge.deleteFolder.ack')}</span>
  </label>
  {#if error}<p role="alert" class="mt-2 text-sm text-danger">{error}</p>{/if}
  {#snippet footer()}
    <Button variant="secondary" onclick={onclose}>{$t('knowledge.editor.cancel')}</Button>
    <Button variant="danger" loading={busy} disabled={!ack || path === ''} onclick={confirm}>{$t('knowledge.deleteFolder.confirm')}</Button>
  {/snippet}
</Dialog>

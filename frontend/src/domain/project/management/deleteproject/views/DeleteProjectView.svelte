<script lang="ts">
  import { tick } from 'svelte';
  import Dialog from '../../../../../shared/components/Dialog.svelte';
  import { isApiError } from '../../../../../shared/services/http';
  import { t } from '../../../../../shared/stores/i18n';
  import { toast } from '../../../../../shared/stores/toast';
  import { reasonOf } from '../../../../../shared/validation/error';
  import DangerZone from '../components/DangerZone.svelte';
  import DeleteProjectForm from '../components/DeleteProjectForm.svelte';
  import { deleteProject } from '../services/deleteproject.service';
  import type { DeletableProject } from '../types';

  let {
    project,
    ondeleted,
    onprojectgone,
    onforbidden,
  }: {
    project: DeletableProject;
    ondeleted: (name: string) => void;
    onprojectgone: () => void;
    onforbidden: () => void;
  } = $props();

  let open = $state(false);
  let typed = $state('');
  let submitting = $state(false);
  let error = $state<string | null>(null);
  let dialogHost: HTMLElement | undefined = $state();

  async function request() {
    typed = '';
    error = null;
    open = true;
    await tick();
    dialogHost?.querySelector<HTMLInputElement>('input[name="confirm_name"]')?.focus();
  }
  function close() {
    open = false;
    typed = '';
    error = null;
  }

  async function submit() {
    submitting = true;
    error = null;
    const name = project.name;
    try {
      await deleteProject(project.id, typed);
      close();
      toast.push({ type: 'success', message: $t('project.delete.success', { name }) });
      ondeleted(name);
    } catch (e) {
      if (!isApiError(e)) {
        error = $t('error.internal');
      } else if (e.status === 400 && reasonOf(e.details) === 'CONFIRMATION_MISMATCH') {
        error = $t('project.error.CONFIRMATION_MISMATCH');
      } else if (e.status === 404) {
        close();
        toast.push({ type: 'error', message: $t('project.delete.alreadyGone') });
        onprojectgone();
      } else if (e.status === 403) {
        close();
        toast.push({ type: 'error', message: $t('error.forbidden') });
        onforbidden();
      } else {
        error = e.traceId ? `${e.message} (${$t('error.trace_id', { id: e.traceId })})` : e.message;
      }
    } finally {
      submitting = false;
    }
  }
</script>

<DangerZone onrequest={request} />
<div bind:this={dialogHost}>
  <Dialog bind:open title={$t('project.delete.title')} onclose={close}>
    <DeleteProjectForm {project} bind:typed {submitting} {error} onsubmit={submit} oncancel={close} />
  </Dialog>
</div>

<script lang="ts">
  import { untrack } from 'svelte';
  import Dialog from '../../../../../shared/components/Dialog.svelte';
  import { isApiError } from '../../../../../shared/services/http';
  import { t } from '../../../../../shared/stores/i18n';
  import { toast } from '../../../../../shared/stores/toast';
  import { fieldErrorsOf } from '../../../../../shared/validation/error';
  import CreateProjectForm from '../components/CreateProjectForm.svelte';
  import { createProject, validateCreate } from '../services/createproject.service';
  import type { CreatedProject, CreateProjectInput } from '../types';

  let {
    open,
    isSuperAdmin,
    onclose,
    oncreated,
  }: {
    open: boolean;
    isSuperAdmin: boolean;
    onclose: () => void;
    oncreated: (p: CreatedProject) => void;
  } = $props();

  let submitting = $state(false);
  let fieldErrors = $state<Record<string, string>>({});
  let formError = $state<string | null>(null);
  let formKey = $state(0);
  let inflight = false;

  // Reset tiap kali dibuka.
  $effect(() => {
    if (open) {
      untrack(() => {
        fieldErrors = {};
        formError = null;
        submitting = false;
        formKey++;
      });
    }
  });

  async function submit(i: CreateProjectInput) {
    if (inflight) return;
    const local = validateCreate(i);
    if (Object.keys(local).length > 0) {
      fieldErrors = Object.fromEntries(Object.entries(local).map(([k, v]) => [k, $t(v)]));
      formError = null;
      return;
    }
    fieldErrors = {};
    formError = null;
    inflight = true;
    submitting = true;
    try {
      const p = await createProject(i);
      toast.push({ type: 'success', message: $t('project.create.success', { name: p.name }) });
      oncreated(p);
    } catch (e) {
      if (isApiError(e)) {
        const fields = e.status === 400 ? fieldErrorsOf(e.details) : {};
        const known = Object.fromEntries(Object.entries(fields).filter(([k]) => k === 'name' || k === 'description'));
        if (Object.keys(known).length > 0) fieldErrors = known;
        else formError = e.traceId ? `${e.message} (${$t('error.trace_id', { id: e.traceId })})` : e.message;
      } else {
        formError = $t('error.internal');
      }
    } finally {
      inflight = false;
      submitting = false;
    }
  }
</script>

<Dialog {open} title={$t('project.create.title')} {onclose}>
  {#key formKey}
    <CreateProjectForm {submitting} {fieldErrors} {formError} showAdminHint={isSuperAdmin} onsubmit={submit} oncancel={onclose} />
  {/key}
</Dialog>

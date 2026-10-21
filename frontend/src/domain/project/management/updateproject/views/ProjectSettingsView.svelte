<script lang="ts">
  import { isApiError } from '../../../../../shared/services/http';
  import { t } from '../../../../../shared/stores/i18n';
  import { toast } from '../../../../../shared/stores/toast';
  import { fieldErrorsOf } from '../../../../../shared/validation/error';
  import ProjectSettingsForm from '../components/ProjectSettingsForm.svelte';
  import { diffPatch, updateProject, validatePatch } from '../services/updateproject.service';
  import type { EditableProject, UpdatedProject } from '../types';

  let {
    project,
    onupdated,
    onprojectgone,
    onforbidden,
  }: {
    project: EditableProject;
    onupdated: (p: UpdatedProject) => void;
    onprojectgone: () => void;
    onforbidden: () => void;
  } = $props();

  let submitting = $state(false);
  let fieldErrors = $state<Record<string, string>>({});
  let formError = $state<string | null>(null);

  async function submit(form: { name: string; description: string }) {
    const patch = diffPatch(project, form);
    if (Object.keys(patch).length === 0) return;
    if (Object.keys(validatePatch(patch)).length > 0) return;
    fieldErrors = {};
    formError = null;
    submitting = true;
    try {
      const p = await updateProject(project.id, patch);
      toast.push({ type: 'success', message: $t('project.settings.saved') });
      onupdated(p);
    } catch (e) {
      if (!isApiError(e)) {
        formError = $t('error.internal');
      } else if (e.status === 400) {
        const f = fieldErrorsOf(e.details);
        const known = Object.fromEntries(Object.entries(f).filter(([k]) => k === 'name' || k === 'description'));
        if (Object.keys(known).length > 0) fieldErrors = known;
        else if (f.body !== undefined) formError = $t('project.settings.nothingToSave');
        else formError = e.message;
      } else if (e.status === 404) {
        toast.push({ type: 'error', message: $t('project.detail.notFound') });
        onprojectgone();
      } else if (e.status === 403) {
        toast.push({ type: 'error', message: $t('error.forbidden') });
        onforbidden();
      } else {
        formError = e.traceId ? `${e.message} (${$t('error.trace_id', { id: e.traceId })})` : e.message;
      }
    } finally {
      submitting = false;
    }
  }
</script>

<section class="flex flex-col gap-4">
  <h2 class="text-lg font-semibold">{$t('project.settings.title')}</h2>
  <ProjectSettingsForm initial={project} {submitting} {fieldErrors} {formError} onsubmit={submit} onreset={() => ((fieldErrors = {}), (formError = null))} />
</section>

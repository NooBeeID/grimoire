<script lang="ts">
  import Button from '../../../../../shared/components/Button.svelte';
  import Dialog from '../../../../../shared/components/Dialog.svelte';
  import { isApiError } from '../../../../../shared/services/http';
  import { t } from '../../../../../shared/stores/i18n';
  import { toast } from '../../../../../shared/stores/toast';
  import { fieldErrorsOf, reasonOf } from '../../../../../shared/validation/error';
  import RenameForm from '../components/RenameForm.svelte';
  import { renameSpec } from '../services/renamespec.service';
  import { validateName, type ApiSpec } from '../types';

  let {
    open,
    projectId,
    spec,
    onclose,
    onrenamed,
    onspecgone,
    onprojectgone,
  }: {
    open: boolean;
    projectId: string;
    spec: { name: string } | null;
    onclose: () => void;
    onrenamed: (spec: ApiSpec) => void;
    onspecgone: () => void;
    onprojectgone: () => void;
  } = $props();

  let value = $state('');
  let submitting = $state(false);
  let error = $state<string | undefined>();
  let formError = $state<string | null>(null);

  $effect(() => {
    if (open) {
      value = spec?.name ?? '';
      error = undefined;
      formError = null;
    }
  });
  const unchanged = $derived(value === (spec?.name ?? ''));

  async function submit() {
    if (!spec || submitting || unchanged) return;
    const v = validateName(value);
    if (v !== 'ok') {
      error = $t(v === 'required' ? 'apispec.validation.nameRequired' : 'apispec.validation.nameInvalid');
      return;
    }
    submitting = true;
    error = undefined;
    formError = null;
    try {
      const res = await renameSpec(projectId, spec.name, value);
      onclose();
      onrenamed(res);
      toast.push({ type: 'success', message: $t('apispec.rename.success', { name: res.name }) });
    } catch (e) {
      if (!isApiError(e)) formError = String(e);
      else if (e.status === 409 && reasonOf(e.details) === 'NAME_TAKEN') error = $t('apispec.error.NAME_TAKEN');
      else if (e.status === 400) error = fieldErrorsOf(e.details).name ?? $t('apispec.error.INVALID_NAME');
      else if (e.status === 404) {
        onclose();
        toast.push({ type: 'error', message: $t('apispec.error.specGone') });
        onspecgone();
      } else formError = e.message;
    } finally {
      submitting = false;
    }
  }
</script>

<Dialog {open} title={$t('apispec.rename.title')} dismissible={!submitting} {onclose}>
  <RenameForm bind:value {projectId} {error} disabled={submitting} onsubmit={submit} />
  {#if formError}<p role="alert" class="mt-3 text-sm text-danger">{formError}</p>{/if}
  {#snippet footer()}
    <Button variant="secondary" disabled={submitting} onclick={onclose}>{$t('common.cancel')}</Button>
    <Button loading={submitting} disabled={unchanged || value === ''} onclick={submit}>{$t('common.save')}</Button>
  {/snippet}
</Dialog>

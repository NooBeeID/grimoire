<script lang="ts">
  import Button from '../../../../../shared/components/Button.svelte';
  import Dialog from '../../../../../shared/components/Dialog.svelte';
  import { isApiError } from '../../../../../shared/services/http';
  import { t } from '../../../../../shared/stores/i18n';
  import { copyText } from '../../../../../shared/utils/clipboard';
  import { fieldErrorsOf, reasonOf } from '../../../../../shared/validation/error';
  import FilePicker from '../components/FilePicker.svelte';
  import NameField from '../components/NameField.svelte';
  import ResultSummary from '../components/ResultSummary.svelte';
  import ResultTable from '../components/ResultTable.svelte';
  import { markOverwrite, rejectedText, validateNameInput } from '../model';
  import { uploadSpecs, validateSpecs } from '../services/uploadspecs.service';
  import { createUploadDialogStore } from '../stores/uploadDialog.store';
  import type { ApiSpecUploadResult } from '../types';

  let {
    open,
    projectId,
    existingNames,
    onclose,
    onuploaded,
    onstale,
    onprojectgone,
  }: {
    open: boolean;
    projectId: string;
    existingNames: Set<string>;
    onclose: () => void;
    onuploaded: (res: ApiSpecUploadResult) => void;
    onstale: () => void;
    onprojectgone: () => void;
  } = $props();

  const store = createUploadDialogStore();
  let name = $state('');
  const busy = $derived($store.phase === 'validating' || $store.phase === 'uploading');
  const canSubmit = $derived($store.spec.length > 0 && !busy);
  const rows = $derived($store.phase === 'validated' ? markOverwrite($store.results, existingNames) : $store.results);
  const hasRejected = $derived($store.results.some((r) => r.status === 'rejected'));

  $effect(() => {
    if (!open) {
      store.reset();
      name = '';
    }
  });
  $effect(() => store.setName(name));

  function nameOk(): boolean {
    const v = validateNameInput(name, $store.spec.length);
    if (v === 'ok') return true;
    store.setNameError($t(`apispec.error.${v}`));
    return false;
  }

  function handleError(e: unknown, uploading: boolean) {
    if (!isApiError(e)) return store.fail({ key: '', message: String(e) });
    const reason = reasonOf(e.details);
    if (e.status === 404) {
      onclose();
      return onprojectgone();
    }
    if (e.code === 'INVALID_RESPONSE' && uploading) {
      store.fail({ key: 'apispec.upload.error.uncertain' });
      return onstale();
    }
    const field = fieldErrorsOf(e.details).name;
    if (e.status === 400 && (reason === 'INVALID_NAME' || field)) {
      store.setNameError(field ?? $t('apispec.error.INVALID_NAME'));
      return store.fail({ key: '' });
    }
    if (e.status === 400 && reason) return store.fail({ key: `apispec.error.${reason}` });
    if (e.status === 413) return store.fail({ key: 'apispec.upload.error.tooLarge' });
    store.fail({ key: '', message: e.message, traceId: e.traceId });
  }

  async function run(kind: 'validate' | 'upload') {
    if (!canSubmit || !nameOk()) return;
    const files = $store.spec;
    const n = name || undefined;
    try {
      if (kind === 'validate') {
        store.startValidate();
        store.validated((await validateSpecs(projectId, files, n)).results);
      } else {
        store.startUpload();
        const res = await uploadSpecs(projectId, files, n);
        store.uploaded(res.results);
        onuploaded(res);
      }
    } catch (e) {
      handleError(e, kind === 'upload');
    }
  }
</script>

<Dialog {open} title={$t('apispec.upload.title')} size="lg" dismissible={!busy} {onclose}>
  <div class="flex flex-col gap-4">
    <FilePicker specCount={$store.spec.length} skippedCount={$store.local.length} disabled={busy} onpick={(f) => store.pick(f)} />
    <NameField bind:value={name} specCount={$store.spec.length} error={$store.nameError ?? undefined} {busy} />
    {#if $store.formError}
      <p role="alert" class="text-sm text-danger">
        {$store.formError.key ? $t($store.formError.key) : ($store.formError.message ?? '')}
        {#if $store.formError.traceId}<span class="block text-xs text-muted">{$t('error.trace_id', { id: $store.formError.traceId })}</span>{/if}
      </p>
    {/if}
    {#if rows.length > 0}
      <ResultSummary results={rows} />
      <ResultTable results={rows} {projectId} linkNames={$store.phase === 'done'} />
    {/if}
  </div>
  {#snippet footer()}
    {#if hasRejected}
      <Button variant="ghost" onclick={() => copyText(rejectedText($store.results))}>{$t('common.copy')}</Button>
    {/if}
    {#if $store.phase === 'done'}
      <Button variant="secondary" onclick={() => store.reset()}>{$t('apispec.upload.again')}</Button>
      <Button onclick={onclose}>{$t('common.close')}</Button>
    {:else}
      <Button variant="secondary" disabled={busy} onclick={onclose}>{$t('common.close')}</Button>
      <Button variant="secondary" disabled={!canSubmit} loading={$store.phase === 'validating'} onclick={() => run('validate')}>
        {$t('apispec.upload.validate')}
      </Button>
      <Button disabled={!canSubmit} loading={$store.phase === 'uploading'} onclick={() => run('upload')}>{$t('apispec.upload.submit')}</Button>
    {/if}
  {/snippet}
</Dialog>

<script lang="ts">
  import Button from '../../../../../shared/components/Button.svelte';
  import Dialog from '../../../../../shared/components/Dialog.svelte';
  import { isApiError } from '../../../../../shared/services/http';
  import { t } from '../../../../../shared/stores/i18n';
  import { fieldErrorsOf, reasonOf } from '../../../../../shared/validation/error';
  import CreateKeyForm from '../components/CreateKeyForm.svelte';
  import KeyResult from '../components/KeyResult.svelte';
  import { createKey } from '../services/createkey.service';
  import { resolveMcpUrl } from '../snippets';
  import { validateName, type CreatedKey } from '../types';

  let {
    open,
    projectId,
    onclose,
    oncreated,
    onprojectgone,
    onstale,
  }: {
    open: boolean;
    projectId: string;
    onclose: () => void;
    oncreated: (key: CreatedKey) => void;
    onprojectgone: () => void;
    onstale: () => void;
  } = $props();

  let step = $state<'form' | 'result'>('form');
  let name = $state('');
  let submitting = $state(false);
  let fieldError = $state<string | null>(null);
  let formError = $state<string | null>(null);
  let traceId = $state<string | undefined>();
  // Raw key hanya hidup di sini dan dibuang saat dialog ditutup.
  let result = $state<{ rawKey: string; name: string; mcpUrl: string } | null>(null);

  function reset() {
    step = 'form';
    name = '';
    fieldError = null;
    formError = null;
    traceId = undefined;
    result = null;
  }
  function close() {
    reset();
    onclose();
  }

  async function submit(raw: string) {
    if (submitting) return;
    const v = validateName(raw);
    fieldError = null;
    formError = null;
    traceId = undefined;
    if (v.error) {
      fieldError = $t(v.error === 'length' ? 'project.apikey.validation.nameLength' : 'project.apikey.validation.nameControl');
      return;
    }
    submitting = true;
    try {
      const res = await createKey(projectId, v.name);
      result = { rawKey: res.raw_key, name: res.key.name, mcpUrl: resolveMcpUrl(res.mcp_url, window.location.origin) };
      step = 'result';
      oncreated(res.key);
    } catch (e) {
      if (!isApiError(e)) {
        formError = String(e);
      } else if (e.status === 404) {
        reset();
        onclose();
        onprojectgone();
      } else if (e.code === 'INVALID_RESPONSE') {
        formError = $t('project.apikey.error.createUncertain');
        onstale();
      } else if (e.status === 400 && fieldErrorsOf(e.details).name) {
        fieldError = fieldErrorsOf(e.details).name!;
      } else if (e.status === 409 && reasonOf(e.details) === 'NAME_TAKEN') {
        fieldError = $t('project.apikey.error.NAME_TAKEN');
      } else if (e.status === 409 && reasonOf(e.details) === 'KEY_LIMIT_REACHED') {
        const limit = (e.details as { limit?: unknown } | null)?.limit;
        formError = typeof limit === 'number' ? $t('project.apikey.error.KEY_LIMIT_REACHED', { limit }) : e.message;
        traceId = typeof limit === 'number' ? undefined : e.traceId;
      } else {
        formError = e.message;
        traceId = e.traceId;
      }
    } finally {
      submitting = false;
    }
  }
</script>

<Dialog
  {open}
  title={step === 'result' ? $t('project.apikey.result.title') : $t('project.apikey.create.title')}
  size={step === 'result' ? 'lg' : 'md'}
  dismissible={!submitting}
  onclose={close}
>
  {#if step === 'result' && result}
    <KeyResult rawKey={result.rawKey} name={result.name} mcpUrl={result.mcpUrl} />
  {:else}
    <CreateKeyForm bind:value={name} {submitting} {fieldError} {formError} {traceId} onsubmit={submit} oncancel={close} />
  {/if}
  {#snippet footer()}
    {#if step === 'result'}<Button onclick={close}>{$t('project.apikey.result.done')}</Button>{/if}
  {/snippet}
</Dialog>

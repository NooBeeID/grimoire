<script lang="ts">
  import Button from '../../../../../shared/components/Button.svelte';
  import Input from '../../../../../shared/components/Input.svelte';
  import { t } from '../../../../../shared/stores/i18n';

  let {
    value = $bindable(''),
    submitting = false,
    fieldError = null,
    formError = null,
    traceId,
    onsubmit,
    oncancel,
  }: {
    value?: string;
    submitting?: boolean;
    fieldError?: string | null;
    formError?: string | null;
    traceId?: string;
    onsubmit: (name: string) => void;
    oncancel?: () => void;
  } = $props();

  function submit(e: SubmitEvent) {
    e.preventDefault();
    if (!submitting) onsubmit(value);
  }
</script>

<form onsubmit={submit} class="flex flex-col gap-4" novalidate>
  <Input
    bind:value
    label={$t('project.apikey.create.name')}
    hint={$t('project.apikey.create.nameHint')}
    error={fieldError ?? undefined}
    disabled={submitting}
    autocomplete="off"
  />
  {#if formError}
    <div role="alert" class="text-sm text-danger">
      <p>{formError}</p>
      {#if traceId}<p class="text-xs text-muted">{$t('error.trace_id', { id: traceId })}</p>{/if}
    </div>
  {/if}
  <div class="flex justify-end gap-2">
    {#if oncancel}<Button variant="secondary" disabled={submitting} onclick={oncancel}>{$t('common.cancel')}</Button>{/if}
    <Button type="submit" loading={submitting}>{$t('project.apikey.create.submit')}</Button>
  </div>
</form>

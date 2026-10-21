<script lang="ts">
  import Button from '../../../../../shared/components/Button.svelte';
  import Input from '../../../../../shared/components/Input.svelte';
  import Textarea from '../../../../../shared/components/Textarea.svelte';
  import { t } from '../../../../../shared/stores/i18n';
  import { runeLength } from '../../../../../shared/utils/text';
  import type { CreateProjectInput } from '../types';

  let {
    submitting = false,
    fieldErrors = {},
    formError = null,
    showAdminHint = false,
    onsubmit,
    oncancel,
  }: {
    submitting?: boolean;
    fieldErrors?: Record<string, string>;
    formError?: string | null;
    showAdminHint?: boolean;
    onsubmit: (i: CreateProjectInput) => void;
    oncancel: () => void;
  } = $props();

  let name = $state('');
  let description = $state('');
  const count = $derived(runeLength(description));

  function submit(e: SubmitEvent) {
    e.preventDefault();
    if (submitting) return;
    onsubmit({ name, description });
  }

  // Fokus ke input pertama yang salah setelah galat divalidasi.
  let form: HTMLFormElement | undefined = $state();
  $effect(() => {
    if (!form || Object.keys(fieldErrors).length === 0) return;
    form.querySelector<HTMLElement>('[aria-invalid="true"]')?.focus();
  });
</script>

<form bind:this={form} onsubmit={submit} class="flex flex-col gap-4" novalidate>
  <Input bind:value={name} name="name" label={$t('project.create.name')} required disabled={submitting} error={fieldErrors.name} />
  <Textarea
    bind:value={description}
    name="description"
    label={$t('project.create.description')}
    disabled={submitting}
    hint={$t('project.create.descriptionCounter', { n: count })}
    error={fieldErrors.description}
  />
  {#if showAdminHint}<p class="text-sm text-muted">{$t('project.create.adminHint')}</p>{/if}
  {#if formError}<p role="alert" class="text-sm text-danger">{formError}</p>{/if}
  <div class="flex justify-end gap-2">
    <Button variant="secondary" onclick={oncancel} disabled={submitting}>{$t('common.cancel')}</Button>
    <Button type="submit" loading={submitting}>{$t('project.create.submit')}</Button>
  </div>
</form>

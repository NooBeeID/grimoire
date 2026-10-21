<script lang="ts">
  import { untrack } from 'svelte';
  import Button from '../../../../../shared/components/Button.svelte';
  import Input from '../../../../../shared/components/Input.svelte';
  import Textarea from '../../../../../shared/components/Textarea.svelte';
  import { t } from '../../../../../shared/stores/i18n';
  import { runeLength } from '../../../../../shared/utils/text';
  import { diffPatch, validatePatch } from '../services/updateproject.service';
  import type { EditableProject } from '../types';

  let {
    initial,
    submitting = false,
    fieldErrors = {},
    formError = null,
    onsubmit,
    onreset,
  }: {
    initial: EditableProject;
    submitting?: boolean;
    fieldErrors?: Record<string, string>;
    formError?: string | null;
    onsubmit: (form: { name: string; description: string }) => void;
    onreset?: () => void;
  } = $props();

  let name = $state(untrack(() => initial.name));
  let description = $state(untrack(() => initial.description));
  let baseName = untrack(() => initial.name);
  let baseDescription = untrack(() => initial.description);

  const patch = $derived(diffPatch(initial, { name, description }));
  const dirty = $derived(Object.keys(patch).length > 0);
  const localErrors = $derived(validatePatch(patch));
  const hasLocalError = $derived(Object.keys(localErrors).length > 0);
  const nameError = $derived(fieldErrors.name ?? (localErrors.name ? $t(localErrors.name) : undefined));
  const descError = $derived(fieldErrors.description ?? (localErrors.description ? $t(localErrors.description) : undefined));

  // Bila initial berubah dari luar: ikuti jika form belum diubah, pertahankan nilai user jika sudah.
  $effect(() => {
    const n = initial.name;
    const d = initial.description;
    untrack(() => {
      if (name === baseName) name = n;
      if (description === baseDescription) description = d;
      baseName = n;
      baseDescription = d;
    });
  });

  function reset() {
    name = initial.name;
    description = initial.description;
    onreset?.();
  }
  function submit(e: SubmitEvent) {
    e.preventDefault();
    if (!dirty || hasLocalError || submitting) return;
    onsubmit({ name, description });
  }
</script>

<form onsubmit={submit} class="flex flex-col gap-4" novalidate>
  <Input bind:value={name} name="name" label={$t('project.settings.name')} required disabled={submitting} error={nameError} />
  <Textarea
    bind:value={description}
    name="description"
    label={$t('project.settings.description')}
    disabled={submitting}
    hint={$t('project.create.descriptionCounter', { n: runeLength(description) })}
    error={descError}
  />
  {#if formError}<p role="alert" class="text-sm text-danger">{formError}</p>{/if}
  <div class="flex justify-end gap-2">
    <Button variant="secondary" onclick={reset} disabled={submitting || !dirty}>{$t('project.settings.reset')}</Button>
    <Button type="submit" loading={submitting} disabled={!dirty || hasLocalError}>{$t('project.settings.save')}</Button>
  </div>
</form>

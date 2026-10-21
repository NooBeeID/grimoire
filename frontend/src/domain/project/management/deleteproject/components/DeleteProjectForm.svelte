<script lang="ts">
  import Button from '../../../../../shared/components/Button.svelte';
  import Input from '../../../../../shared/components/Input.svelte';
  import { t } from '../../../../../shared/stores/i18n';
  import { confirmMatches } from '../services/deleteproject.service';
  import type { DeletableProject } from '../types';

  let {
    project,
    typed = $bindable(''),
    submitting = false,
    error = null,
    onsubmit,
    oncancel,
  }: {
    project: DeletableProject;
    typed?: string;
    submitting?: boolean;
    error?: string | null;
    onsubmit: () => void;
    oncancel: () => void;
  } = $props();

  const n = (v: number | null) => (v === null ? '-' : String(v));
  const ok = $derived(confirmMatches(typed, project.name));

  function submit(e: SubmitEvent) {
    e.preventDefault();
    if (ok && !submitting) onsubmit();
  }
</script>

<form onsubmit={submit} class="flex flex-col gap-4">
  <p class="text-sm">
    {$t('project.delete.impact', {
      members: project.member_count,
      files: n(project.counts.files),
      specs: n(project.counts.specs),
      keys: n(project.counts.keys),
    })}
  </p>
  <p class="text-sm">{$t('project.delete.typeToConfirm')} <code class="rounded-base bg-black/5 px-1 break-all">{project.name}</code></p>
  <Input bind:value={typed} name="confirm_name" label={$t('project.delete.confirmLabel')} autocomplete="off" disabled={submitting} />
  {#if error}<p role="alert" class="text-sm text-danger">{error}</p>{/if}
  <div class="flex justify-end gap-2">
    <Button variant="secondary" onclick={oncancel} disabled={submitting}>{$t('common.cancel')}</Button>
    <Button type="submit" variant="danger" disabled={!ok} loading={submitting}>{$t('project.delete.submit')}</Button>
  </div>
</form>

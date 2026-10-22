<script lang="ts">
  import Input from '../../../../../shared/components/Input.svelte';
  import { t } from '../../../../../shared/stores/i18n';
  import { parseRepoUrlLite, type Kind } from '../../base';
  import type { FormTouched, FormValues } from '../types';

  let {
    kind,
    values = $bindable(),
    touched = $bindable(),
    errors = {},
    disabled = false,
  }: {
    kind: Kind;
    values: FormValues;
    touched: FormTouched;
    errors?: { url?: string; branch?: string; folder?: string; target_prefix?: string };
    disabled?: boolean;
  } = $props();

  // URL tree mengisi Branch/Folder hanya untuk field yang belum disentuh pengguna.
  function onUrlInput() {
    const p = parseRepoUrlLite(values.url);
    if (!p.ok) return;
    if (p.branch !== undefined && !touched.branch) values.branch = p.branch;
    if (p.folder !== undefined && !touched.folder) values.folder = p.folder;
  }
</script>

<div class="flex flex-col gap-4">
  <div oninput={onUrlInput} role="presentation">
    <Input bind:value={values.url} name="git-url" autocomplete="off" label={$t('gitSync.field.url')} hint={$t('gitSync.help.url')} error={errors.url} {disabled} />
  </div>
  <div class="grid gap-4 sm:grid-cols-2">
    <div oninput={() => (touched.branch = true)} role="presentation">
      <Input bind:value={values.branch} name="git-branch" autocomplete="off" label={$t('gitSync.field.branch')} hint={$t('gitSync.help.branch')} error={errors.branch} {disabled} />
    </div>
    <div oninput={() => (touched.folder = true)} role="presentation">
      <Input bind:value={values.folder} name="git-folder" autocomplete="off" label={$t('gitSync.field.folder')} hint={$t('gitSync.help.folder')} error={errors.folder} {disabled} />
    </div>
  </div>
  {#if kind === 'knowledge'}
    <Input
      bind:value={values.targetPrefix}
      name="git-target"
      autocomplete="off"
      label={$t('gitSync.field.targetPrefix')}
      placeholder={$t('gitSync.targetPrefixPlaceholder')}
      hint={$t('gitSync.help.targetPrefix')}
      error={errors.target_prefix}
      {disabled}
    />
  {/if}
</div>

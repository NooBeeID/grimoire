<script lang="ts">
  import Button from '../../../../../shared/components/Button.svelte';
  import { t } from '../../../../../shared/stores/i18n';
  import type { PickedFile } from '../types';

  let {
    specCount,
    skippedCount,
    disabled = false,
    onpick,
  }: { specCount: number; skippedCount: number; disabled?: boolean; onpick: (files: PickedFile[]) => void } = $props();

  let fileInput: HTMLInputElement | undefined = $state();
  let folderInput: HTMLInputElement | undefined = $state();

  function handle(e: Event) {
    const el = e.currentTarget as HTMLInputElement;
    const files = [...(el.files ?? [])].map((file) => ({ file, path: file.webkitRelativePath || file.name }));
    el.value = '';
    onpick(files);
  }
</script>

<div class="flex flex-col gap-3 rounded-card border border-dashed border-control p-4">
  <p class="text-sm text-muted">{$t('apispec.upload.pickHint')}</p>
  <div class="flex flex-wrap gap-2">
    <Button variant="secondary" {disabled} onclick={() => fileInput?.click()}>{$t('apispec.upload.pickFiles')}</Button>
    <Button variant="secondary" {disabled} onclick={() => folderInput?.click()}>{$t('apispec.upload.pickFolder')}</Button>
  </div>
  <input
    bind:this={fileInput}
    type="file"
    multiple
    class="hidden"
    tabindex="-1"
    accept=".yaml,.yml,.json,application/json,application/yaml"
    data-testid="apispec-file-input"
    onchange={handle}
  />
  <input bind:this={folderInput} type="file" multiple class="hidden" tabindex="-1" data-testid="apispec-folder-input" onchange={handle} {...{ webkitdirectory: true }} />
  {#if specCount + skippedCount > 0}
    <p class="text-sm" aria-live="polite">
      {specCount > 0 ? $t('apispec.upload.selected', { spec: specCount, skipped: skippedCount }) : $t('apispec.upload.noSpecFiles')}
    </p>
  {/if}
</div>

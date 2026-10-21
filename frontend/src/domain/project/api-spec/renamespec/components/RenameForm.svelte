<script lang="ts">
  import Input from '../../../../../shared/components/Input.svelte';
  import { t } from '../../../../../shared/stores/i18n';

  let {
    value = $bindable(''),
    projectId,
    error,
    disabled = false,
    onsubmit,
  }: { value?: string; projectId: string; error?: string; disabled?: boolean; onsubmit: () => void } = $props();
</script>

<form
  class="flex flex-col gap-3"
  onsubmit={(e) => {
    e.preventDefault();
    onsubmit();
  }}
>
  <Input bind:value label={$t('apispec.rename.label')} hint={$t('apispec.validation.nameInvalid')} {error} {disabled} autocomplete="off" />
  {#if value}
    <p class="font-mono text-xs break-all text-muted">
      {$t('apispec.rename.preview', { url: `/projects/${projectId}/api-specs/${value}` })}
    </p>
  {/if}
</form>

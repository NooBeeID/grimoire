<script lang="ts">
  import Input from '../../../../../shared/components/Input.svelte';
  import { t } from '../../../../../shared/stores/i18n';

  // Nama hanya bisa diisi saat tepat satu file spec dipilih.
  let {
    value = $bindable(''),
    specCount,
    error,
    busy = false,
  }: { value?: string; specCount: number; error?: string; busy?: boolean } = $props();
  const single = $derived(specCount === 1);
</script>

<Input
  bind:value
  label={$t('apispec.upload.name')}
  hint={single ? $t('apispec.upload.nameHint') : $t('apispec.upload.nameOnlySingle')}
  {error}
  disabled={busy || !single}
  autocomplete="off"
/>

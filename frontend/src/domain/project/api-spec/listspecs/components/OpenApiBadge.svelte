<script lang="ts">
  import { t } from '../../../../../shared/stores/i18n';
  import { openapiMajorMinor } from '../types';

  let { version }: { version: string } = $props();
  const short = $derived(openapiMajorMinor(version));
</script>

<span
  class="inline-flex h-5 items-center rounded-full bg-pill-neutral-bg px-2 text-xs font-medium text-pill-neutral-ink"
  title={version}
  aria-label="{$t('apispec.openapiVersion')} {version}">{short}</span
>

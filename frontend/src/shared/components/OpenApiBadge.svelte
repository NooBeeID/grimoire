<script lang="ts">
  import { t } from '../stores/i18n';

  let { version }: { version: string } = $props();
  const short = $derived(/^(\d+\.\d+)(\.|$)/.exec(version)?.[1] ?? version); // "3.1.0" -> "3.1"
</script>

<span
  class="inline-flex h-5 items-center rounded-full bg-pill-neutral-bg px-2 text-xs font-medium text-pill-neutral-ink"
  title={version}
  aria-label="{$t('apispec.openapiVersion')} {version}">{short}</span
>

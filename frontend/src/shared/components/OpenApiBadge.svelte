<script lang="ts">
  import { t } from '../stores/i18n';
  import { isSwagger2Version } from '../utils/openapi';

  let { version }: { version: string } = $props();
  const swagger2 = $derived(isSwagger2Version(version));
  const short = $derived(/^(\d+\.\d+)(\.|$)/.exec(version)?.[1] ?? version); // "3.1.0" -> "3.1"
</script>

<span
  class="inline-flex h-5 items-center rounded-full bg-pill-neutral-bg px-2 text-xs font-medium text-pill-neutral-ink"
  title={swagger2 ? 'Swagger 2.0' : version}
  aria-label={swagger2 ? 'Swagger 2.0' : `${$t('apispec.openapiVersion')} ${version}`}>{swagger2 ? 'Swagger 2.0' : short}</span
>

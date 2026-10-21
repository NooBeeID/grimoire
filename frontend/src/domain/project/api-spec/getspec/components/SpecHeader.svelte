<script lang="ts">
  import type { Snippet } from 'svelte';
  import OpenApiBadge from '../../../../../shared/components/OpenApiBadge.svelte';
  import { locale, t } from '../../../../../shared/stores/i18n';
  import { formatDateTime } from '../../../../../shared/utils/datetime';
  import type { ApiSpec } from '../types';

  let { spec, actions }: { spec: ApiSpec; actions?: Snippet<[ApiSpec]> } = $props();

  const size = $derived(
    new Intl.NumberFormat($locale === 'id' ? 'id-ID' : 'en-US', { maximumFractionDigits: 1 }).format(spec.size_bytes / 1024) + ' KB',
  );
</script>

<header class="flex flex-wrap items-start justify-between gap-3">
  <div class="min-w-0">
    <h2 class="font-mono text-[22px] leading-tight break-all">{spec.name}</h2>
    <p class="mt-1 flex flex-wrap items-center gap-2 text-sm">{spec.title} <OpenApiBadge version={spec.openapi_version} /> <span>{spec.api_version}</span></p>
    <p class="mt-1 text-xs text-muted">
      {$t('apispec.detail.meta', { format: spec.format, size, count: spec.endpoint_count, source: spec.source_path ?? '–' })}
    </p>
    <p class="text-xs text-muted">
      {$t('apispec.detail.updatedBy', { by: spec.updated_by_label ?? '–', at: formatDateTime(spec.updated_at, $locale) })}
    </p>
  </div>
  {#if actions}<div class="flex flex-wrap items-center gap-2">{@render actions(spec)}</div>{/if}
</header>

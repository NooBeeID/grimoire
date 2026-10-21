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

<header class="flex flex-col gap-4">
  <div class="flex flex-wrap items-start justify-between gap-3">
    <div class="min-w-0">
      <h2 class="font-mono text-[22px] leading-tight break-all">{spec.name}</h2>
      <p class="mt-1.5 flex flex-wrap items-center gap-2 text-sm">
        <span>{spec.title}</span>
        <OpenApiBadge version={spec.openapi_version} />
        <span class="text-muted">{spec.api_version}</span>
      </p>
    </div>
    {#if actions}<div class="flex flex-wrap items-center gap-2">{@render actions(spec)}</div>{/if}
  </div>

  <dl class="grid grid-cols-2 overflow-hidden rounded-card border border-border bg-surface sm:grid-cols-4 [&>div]:px-4 [&>div]:py-3 sm:[&>div+div]:border-l sm:[&>div+div]:border-rule">
    <div>
      <dt class="text-xs text-muted">{$t('apispec.detail.metaEndpoints')}</dt>
      <dd class="mt-1 text-[28px] leading-8 font-normal tabular-nums">{spec.endpoint_count}</dd>
    </div>
    <div>
      <dt class="text-xs text-muted">{$t('apispec.detail.metaFormat')}</dt>
      <dd class="mt-1 text-base leading-8 font-medium">{spec.format}</dd>
    </div>
    <div>
      <dt class="text-xs text-muted">{$t('apispec.detail.metaSize')}</dt>
      <dd class="mt-1 text-base leading-8 font-medium tabular-nums">{size}</dd>
    </div>
    <div class="min-w-0">
      <dt class="text-xs text-muted">{$t('apispec.detail.metaSource')}</dt>
      <dd class="mt-1 truncate font-mono text-[13px] leading-8" title={spec.source_path ?? undefined}>{spec.source_path ?? '–'}</dd>
    </div>
  </dl>
  <p class="text-xs text-muted">
    {$t('apispec.detail.updatedBy', { by: spec.updated_by_label ?? '–', at: formatDateTime(spec.updated_at, $locale) })}
  </p>
</header>

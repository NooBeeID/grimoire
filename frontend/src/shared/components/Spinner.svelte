<script lang="ts">
  import { t } from '../stores/i18n';
  let { size = 20, label }: { size?: number; label?: string } = $props();
</script>

<span role="status" class="inline-flex items-center">
  <svg
    class="animate-spin motion-reduce:animate-none"
    width={size}
    height={size}
    viewBox="0 0 24 24"
    fill="none"
    aria-hidden="true"
  >
    <circle cx="12" cy="12" r="10" stroke="currentColor" stroke-opacity="0.25" stroke-width="4" />
    <path d="M22 12a10 10 0 0 0-10-10" stroke="currentColor" stroke-width="4" stroke-linecap="round" />
  </svg>
  <span class="sr-only">{label ?? $t('common.loading')}</span>
</span>

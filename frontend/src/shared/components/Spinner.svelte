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
    <path d="M12 2.5 20.2 7.25v9.5L12 21.5 3.8 16.75v-9.5Z" stroke="currentColor" stroke-opacity="0.25" stroke-width="3" stroke-linejoin="round" />
    <path d="M12 2.5 20.2 7.25v4.2" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round" />
  </svg>
  <span class="sr-only">{label ?? $t('common.loading')}</span>
</span>

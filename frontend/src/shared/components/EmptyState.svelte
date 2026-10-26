<script lang="ts">
  import type { Snippet } from 'svelte';
  import { t } from '../stores/i18n';
  let { title, description, action }: { title?: string; description?: string; action?: Snippet } = $props();
</script>

<div class="flex flex-col items-center gap-1.5 px-6 py-14 text-center">
  <h3 class="text-[15px] font-semibold">{title ?? $t('common.empty')}</h3>
  {#if description}<p class="max-w-sm text-sm text-muted">{description}</p>{/if}
  {#if action}<div class="mt-3">{@render action()}</div>{/if}
</div>

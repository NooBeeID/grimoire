<script lang="ts">
  import { t } from '../../../../../shared/stores/i18n';
  import { SUMMARY_RANGES, type SummaryRange } from '../types';

  let { value, onchange }: { value: SummaryRange; onchange: (r: SummaryRange) => void } = $props();
  const uid = $props.id();
</script>

<fieldset class="m-0 min-w-0 border-0 p-0">
  <legend class="sr-only">{$t('dashboard.range.label')}</legend>
  <div class="inline-flex h-8 rounded-base bg-raised p-0.5 max-[480px]:flex max-[480px]:w-full">
    {#each SUMMARY_RANGES as r (r)}
      <label class="relative flex-1">
        <input
          type="radio"
          name="range-{uid}"
          value={r}
          checked={value === r}
          onchange={() => onchange(r)}
          class="peer sr-only"
        />
        <span
          class="inline-flex h-7 cursor-pointer items-center justify-center rounded-base px-3 text-sm whitespace-nowrap text-muted peer-checked:bg-surface peer-checked:font-medium peer-checked:text-fg peer-checked:shadow-xs peer-focus-visible:ring-2 peer-focus-visible:ring-accent-ink"
        >
          {$t(`dashboard.range.${r}`)}
        </span>
      </label>
    {/each}
  </div>
</fieldset>

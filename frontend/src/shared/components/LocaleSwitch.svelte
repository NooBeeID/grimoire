<script lang="ts">
  import { SUPPORTED_LOCALES, locale, setLocale, t, type Locale } from '../stores/i18n';

  const uid = $props.id();
  const id = `loc-${uid}`;

  function onchange(e: Event) {
    setLocale((e.currentTarget as HTMLSelectElement).value as Locale);
  }
</script>

<label for={id} class="sr-only">{$t('locale.label')}</label>
<select
  {id}
  value={$locale}
  {onchange}
  class="rounded-base border border-border bg-surface px-2 py-1 text-sm focus-visible:ring-2 focus-visible:ring-primary focus-visible:outline-none"
>
  {#each SUPPORTED_LOCALES as l (l)}
    <option value={l}>{$t(`locale.${l}`)}</option>
  {/each}
</select>

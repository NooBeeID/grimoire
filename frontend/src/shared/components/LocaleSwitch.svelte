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
  class="outlined h-9 w-full rounded-full bg-surface/70 px-3 text-[13px] font-medium"
>
  {#each SUPPORTED_LOCALES as l (l)}
    <option value={l}>{$t(`locale.${l}`)}</option>
  {/each}
</select>

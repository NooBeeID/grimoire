<script lang="ts">
  import { SUPPORTED_LOCALES, locale, setLocale, t, type Locale } from '../stores/i18n';

  const uid = $props.id();
  const id = `loc-${uid}`;

  function onchange(e: Event) {
    setLocale((e.currentTarget as HTMLSelectElement).value as Locale);
  }
</script>

<label for={id} class="sr-only">{$t('locale.label')}</label>
<div class="relative md:w-9">
<select
  {id}
  value={$locale}
  {onchange}
  title={$t('locale.label')}
  class="outlined h-9 w-full rounded-full bg-surface/70 px-3 text-[13px] font-medium md:appearance-none md:px-0 md:text-transparent md:[&_option]:text-fg"
>
  {#each SUPPORTED_LOCALES as l (l)}
    <option value={l}>{$t(`locale.${l}`)}</option>
  {/each}
</select>
<!-- Desktop (sidebar hanya ikon): tampilkan kode bahasa saja di atas select. -->
<span aria-hidden="true" class="pointer-events-none absolute inset-0 hidden place-items-center rounded-full text-[11px] font-semibold text-muted uppercase md:grid">{$locale}</span>
</div>

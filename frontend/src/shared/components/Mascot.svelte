<script lang="ts">
  // Tanda merek Grimoire: heksagon madu dengan titik ink. Dekoratif (aria-hidden) kecuali diberi label.
  // Nama dan props lama dipertahankan agar pemanggil tidak berubah; mood/alive tidak lagi berpengaruh.
  let {
    size = 24,
    label,
  }: {
    mood?: 'happy' | 'confused' | 'oops' | 'sleepy';
    size?: number;
    alive?: boolean;
    label?: string;
  } = $props();
</script>

<svg
  viewBox="0 0 24 24"
  width={size}
  height={size}
  class="shrink-0"
  role={label ? 'img' : undefined}
  aria-label={label}
  aria-hidden={label ? undefined : 'true'}
>
  <path d="M12 2.2 20.5 7v10L12 21.8 3.5 17V7Z" fill="var(--color-brand)" />
  <circle cx="12" cy="12" r="3.1" fill="#0f1216" />
</svg>

<script lang="ts">
  // Identitas pucat dari nama: inisial + hue stabil. Hue dipilih dari enam nilai yang berjarak >= 50 derajat,
  // jadi dua orang yang berdampingan jarang sama. Bukan data: tidak pernah dipakai di chart atau status.
  let {
    name,
    size = 28,
    shape = 'circle',
  }: { name: string; size?: number; shape?: 'circle' | 'square' } = $props();

  const HUES = [205, 265, 330, 25, 85, 150];
  const hue = $derived.by(() => {
    let h = 0;
    for (const ch of name) h = (h * 31 + ch.charCodeAt(0)) >>> 0;
    return HUES[h % HUES.length];
  });
  const initial = $derived((name.trim().charAt(0) || '?').toUpperCase());
</script>

<span
  aria-hidden="true"
  class="avatar inline-grid shrink-0 place-items-center font-semibold select-none {shape === 'circle' ? 'rounded-full' : 'rounded-card'}"
  style="--h: {hue}; width: {size}px; height: {size}px; font-size: {Math.round(size * 0.42)}px"
>{initial}</span>

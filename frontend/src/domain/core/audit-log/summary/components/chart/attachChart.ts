import type { Attachment } from 'svelte/attachments';
import { Chart, type ChartConfiguration } from 'chart.js';
import { setupCharts } from './chartSetup';

/**
 * Attachment Svelte 5. Dijalankan ulang saat state reaktif di getConfig berubah (data, locale, tema):
 * chart lama dihancurkan lalu dibuat lagi, sehingga tidak ada state kedua yang perlu disinkronkan.
 */
export function chart(getConfig: () => ChartConfiguration): Attachment<HTMLCanvasElement> {
  return (canvas) => {
    setupCharts();
    const instance = new Chart(canvas, getConfig());
    return () => instance.destroy();
  };
}

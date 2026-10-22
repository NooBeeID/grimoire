import {
  BarController,
  BarElement,
  CategoryScale,
  Chart,
  Legend,
  LinearScale,
  LineController,
  LineElement,
  PointElement,
  Tooltip,
} from 'chart.js';

let ready = false;

// Hanya sembilan komponen yang dipakai (tidak memakai chart.js/auto); dipanggil sekali.
export function setupCharts(): typeof Chart {
  if (ready) return Chart;
  Chart.register(
    BarController,
    LineController,
    BarElement,
    LineElement,
    PointElement,
    CategoryScale,
    LinearScale,
    Tooltip,
    Legend,
  );
  Chart.defaults.animation = false;
  Chart.defaults.responsive = true;
  Chart.defaults.maintainAspectRatio = false;
  if (typeof document !== 'undefined') {
    Chart.defaults.font.family = getComputedStyle(document.body).fontFamily || 'sans-serif';
  }
  Chart.defaults.font.size = 12;
  ready = true;
  return Chart;
}

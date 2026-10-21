// Menentukan tema sebelum render pertama (tanpa kedip). Berkas eksternal karena CSP hanya mengizinkan script-src 'self'.
// Logika yang sama ada di src/shared/stores/theme.ts; ubah keduanya bersama.
(function () {
  var theme = 'light';
  try {
    var stored = localStorage.getItem('grimoire.theme');
    var dark = stored === 'dark' || (stored !== 'light' && window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches);
    theme = dark ? 'dark' : 'light';
  } catch (e) {
    // storage tidak tersedia: ikuti terang
  }
  document.documentElement.dataset.theme = theme;
})();

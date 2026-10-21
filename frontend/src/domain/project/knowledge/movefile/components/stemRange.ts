// Rentang [mulai, akhir) nama file tanpa ekstensi pada path penuh, untuk memilih bagian
// yang paling sering diganti saat rename. Tanpa ekstensi: seluruh nama file.
export function stemRange(path: string): [number, number] {
  const slash = path.lastIndexOf('/');
  const start = slash + 1;
  const dot = path.lastIndexOf('.');
  const end = dot > start ? dot : path.length;
  return [start, end];
}

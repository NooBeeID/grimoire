// Menyimpan Blob sebagai file lewat <a download> sementara. Nama file disaring agar aman.
export function saveBlob(blob: Blob, fileName: string): void {
  const safe = fileName.replace(/[^A-Za-z0-9._-]/g, '_');
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = safe;
  a.rel = 'noopener';
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 0);
}

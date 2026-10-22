export function remainingSeconds(until: number, now: number): number {
  return Math.max(0, Math.ceil((until - now) / 1000));
}

export function percentOf(done: number, total: number): number {
  if (total <= 0) return 100;
  return Math.min(100, Math.max(0, Math.floor((done / total) * 100)));
}

// Potong di tengah path; nama file selalu utuh.
export function truncateMiddle(path: string, max = 48): string {
  if (path.length <= max) return path;
  const i = path.lastIndexOf('/');
  const file = i === -1 ? path : path.slice(i + 1);
  if (i === -1) return path;
  const head = path.slice(0, path.indexOf('/') + 1);
  const rest = `${head}…/${file}`;
  if (rest.length >= path.length) return path;
  const room = max - (1 + 1 + file.length); // '…' + '/' + file
  if (room <= head.length) return rest;
  const dir = path.slice(0, i);
  return `${dir.slice(0, room)}…/${file}`;
}

export function announceBucket(seconds: number): number {
  return Math.ceil(seconds / 5) * 5;
}

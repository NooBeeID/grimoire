package base

// CheckBatchLimits menolak batch utuh bila jumlah kandidat > MaxFiles atau jumlah ukuran
// yang diketahui (Size >= 0) > MaxTotalBytes (D3). Dipanggil dari tree, sebelum fetch.
func CheckBatchLimits(cands []Candidate, l Limits) error {
	var known int64
	for _, c := range cands {
		if c.Size >= 0 {
			known += c.Size
		}
	}
	if len(cands) > l.MaxFiles {
		return SyncLimitFiles(len(cands), l, known)
	}
	if known > l.MaxTotalBytes {
		return SyncLimitBytes(len(cands), l, known)
	}
	return nil
}

// CheckFetchedTotal memeriksa total byte kumulatif saat fetch.
func CheckFetchedTotal(total int64, l Limits) error {
	if total > l.MaxTotalBytes {
		return SyncLimitBytes(0, l, total)
	}
	return nil
}

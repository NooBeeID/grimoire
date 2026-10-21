package record

// Batas yang diterapkan provider core.audit.record. Consumer memakai konstanta ini, bukan menyalin angka.
const (
	MaxTargetBytes     = 1024 // Target dipotong ke batas ini (batas UTF-8)
	MaxResultJSONBytes = 8192 // Args dan Result masing-masing; lewat batas -> diganti {_truncated,...}
)

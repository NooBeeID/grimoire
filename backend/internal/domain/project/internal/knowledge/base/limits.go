package base

import "grimoire/internal/infra/config"

// Limits adalah batas ukuran/jumlah module, diturunkan dari config.
type Limits struct {
	MaxFileBytes    int64
	MaxUploadBytes  int64
	MaxUploadFiles  int
	MaxProjectFiles int
	MaxReadBytes    int // = Config.MCPMaxResponseBytes (clamp read-file)
}

func LimitsFrom(c config.Config) Limits {
	return Limits{
		MaxFileBytes: c.MaxFileBytes, MaxUploadBytes: c.MaxUploadBytes, MaxUploadFiles: c.MaxUploadFiles,
		MaxProjectFiles: c.MaxProjectFiles, MaxReadBytes: c.MCPMaxResponseBytes,
	}
}

// JSONBodyLimit: escape JSON terburuk (\u00XX = 6 byte per byte) + ruang untuk field lain.
func (l Limits) JSONBodyLimit() int64 { return 6*l.MaxFileBytes + 64<<10 }

// UploadBodyLimit: total upload + overhead header multipart.
func (l Limits) UploadBodyLimit() int64 { return l.MaxUploadBytes + 1<<20 }

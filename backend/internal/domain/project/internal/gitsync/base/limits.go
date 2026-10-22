package base

import "grimoire/internal/infra/config"

// Limits adalah batas satu sync (D3).
type Limits struct {
	MaxFileBytes  int64
	MaxFiles      int
	MaxTotalBytes int64
}

// LimitsFor memetakan config ke batas per kind. api_spec: MaxFiles = min(MaxUploadFiles, APISpecMaxSpecs).
func LimitsFor(k Kind, cfg config.Config) Limits {
	if k == KindAPISpec {
		return Limits{
			MaxFileBytes:  cfg.APISpecMaxFileBytes,
			MaxFiles:      min(cfg.MaxUploadFiles, cfg.APISpecMaxSpecs),
			MaxTotalBytes: cfg.MaxUploadBytes,
		}
	}
	return Limits{MaxFileBytes: cfg.MaxFileBytes, MaxFiles: cfg.MaxUploadFiles, MaxTotalBytes: cfg.MaxUploadBytes}
}

package base

import (
	"testing"

	"grimoire/internal/infra/config"
)

func TestLimits(t *testing.T) {
	l := LimitsFrom(config.Config{
		MaxFileBytes: 1 << 20, MaxUploadBytes: 50 << 20, MaxUploadFiles: 500, MaxProjectFiles: 5000, MCPMaxResponseBytes: 102400,
	})
	if l.MaxFileBytes != 1<<20 || l.MaxUploadBytes != 50<<20 || l.MaxUploadFiles != 500 || l.MaxProjectFiles != 5000 || l.MaxReadBytes != 102400 {
		t.Fatalf("%+v", l)
	}
	if l.JSONBodyLimit() != 6*(1<<20)+64<<10 {
		t.Fatalf("json limit %d", l.JSONBodyLimit())
	}
	if l.UploadBodyLimit() != 50<<20+1<<20 {
		t.Fatalf("upload limit %d", l.UploadBodyLimit())
	}
}

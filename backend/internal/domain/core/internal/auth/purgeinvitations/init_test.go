package purgeinvitations

import (
	"context"
	"testing"
)

func TestNewJob(t *testing.T) {
	j := newJob(func(context.Context) error { return nil })
	if j.Name != "core.auth.purge-invitations" {
		t.Fatalf("name = %q", j.Name)
	}
	if j.DailyAt != "03:30" {
		t.Fatalf("dailyAt = %q", j.DailyAt)
	}
	if j.Run == nil {
		t.Fatal("Run nil")
	}
}

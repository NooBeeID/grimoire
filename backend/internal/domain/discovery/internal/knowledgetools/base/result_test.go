package base

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"grimoire/lib/common/errs"
)

func TestArgResult(t *testing.T) {
	r := ArgResult(&ArgError{Field: "limit", Reason: "harus 1-5"}, "folder", "docs")
	if !r.IsError || r.TargetType != "folder" || r.Target != "docs" || r.Summary["field"] != "limit" || !strings.Contains(r.Text, "limit harus 1-5") {
		t.Fatalf("%+v", r)
	}
}

func TestBusResult(t *testing.T) {
	r, err := BusResult("read_file", fmt.Errorf("x: %w", errs.ErrInvalid), "Coba lagi.", "file", "a")
	if err != nil || !r.IsError || r.Summary["error_code"] != "invalid_request" ||
		!strings.HasPrefix(r.Text, "Permintaan ditolak: x: invalid argument. Coba lagi.") || r.Target != "a" {
		t.Fatalf("%+v %v", r, err)
	}
	for _, in := range []error{context.DeadlineExceeded, errors.New("not registered")} {
		_, err := BusResult("read_file", in, "", "file", "a")
		if err == nil || !errors.Is(err, in) || !strings.HasPrefix(err.Error(), "read_file: ") {
			t.Fatalf("%v", err)
		}
	}
}

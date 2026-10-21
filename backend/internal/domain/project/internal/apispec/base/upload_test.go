package base

import (
	"strings"
	"testing"
)

func TestDecideUpload(t *testing.T) {
	h1, h2 := HashContent("a"), HashContent("b")
	ex := &Spec{Hash: h1}
	cases := []struct {
		name     string
		existing *Spec
		h        [32]byte
		count    int
		max      int
		want     UploadDecision
	}{
		{"create", nil, h1, 1, 5, DecisionCreate},
		{"reject limit", nil, h1, 5, 5, DecisionRejectLimit},
		{"update at limit", ex, h2, 5, 5, DecisionUpdate},
		{"unchanged at limit", ex, h1, 5, 5, DecisionUnchanged},
		{"update", ex, h2, 1, 5, DecisionUpdate},
	}
	for _, c := range cases {
		if got := DecideUpload(c.existing, c.h, c.count, c.max); got != c.want {
			t.Errorf("%s: got %d want %d", c.name, got, c.want)
		}
	}
}

func TestReasonMessage(t *testing.T) {
	if m := ReasonMessage(ReasonFileTooLarge, 100); !strings.Contains(m, "100 byte") {
		t.Fatal(m)
	}
	if m := ReasonMessage(ReasonSpecLimitReached, 7); !strings.Contains(m, "7 spec") {
		t.Fatal(m)
	}
	if ReasonMessage(ReasonNotSpecFile) == "" || ReasonMessage("ZZZ") != "ZZZ" {
		t.Fatal("fallback")
	}
	for code := range Reasons {
		if ReasonMessage(code, 1) == "" {
			t.Fatal(code)
		}
	}
}

func TestSummarize(t *testing.T) {
	rs := []FileResult{{Status: StatusCreated}, {Status: StatusCreated}, {Status: StatusUpdated}, {Status: StatusUnchanged},
		{Status: StatusSkipped}, {Status: StatusRejected}, {Status: StatusRejected}, {Status: StatusValid}}
	got := Summarize(rs)
	want := UploadSummary{Created: 2, Updated: 1, Unchanged: 1, Skipped: 1, Rejected: 2}
	if got != want {
		t.Fatalf("%+v", got)
	}
}

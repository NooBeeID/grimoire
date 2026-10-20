package archtest

import (
	"strings"
	"testing"
)

const (
	core    = "grimoire/internal/domain/core"
	project = "grimoire/internal/domain/project"
)

func TestCheck(t *testing.T) {
	cases := []struct {
		name     string
		from, to string
		rule     string // "" = diizinkan
	}{
		{"domain->domain", core + "/internal/auth", project + "/internal/apikey", Rule41},
		{"domain root->domain lain", core, project, Rule41},
		{"module->module", project + "/internal/apikey", project + "/internal/knowledge/base", Rule42},
		{"feature->feature", project + "/internal/knowledge/upload", project + "/internal/knowledge/search", Rule43},
		{"subpackage feature->feature", project + "/internal/knowledge/upload/repository", project + "/internal/knowledge/search", Rule43},
		{"infra->domain", "grimoire/internal/infra/bus", core, Rule46},
		{"contracts->infra", "grimoire/lib/contracts/core", "grimoire/internal/infra/bus", Rule45},
		{"contracts->cmd", "grimoire/lib/contracts/core", "grimoire/cmd/http", Rule45},
		{"contracts->constants", "grimoire/lib/contracts/core", "grimoire/lib/constants", Rule45},
		{"contracts->util", "grimoire/lib/contracts/core", "grimoire/lib/util", Rule45},
		{"lib->infra", "grimoire/lib/util", "grimoire/internal/infra/bus", Rule49},
		{"lib->archtest", "grimoire/lib/common/errs", "grimoire/internal/archtest", Rule49},
		{"Rule44 consumer->provider module", core + "/internal/auth/login", project + "/internal/management", Rule41},
		{"Rule44 consumer->provider module sama domain", project + "/internal/apikey/create", project + "/internal/management", Rule42},

		{"domain->infra ok", core + "/internal/auth", "grimoire/internal/infra/bus", ""},
		{"domain->lib ok", core + "/internal/auth", "grimoire/lib/contracts/core/x", ""},
		{"domain->constants ok", core + "/internal/auth", "grimoire/lib/constants", ""},
		{"cmd->apa saja ok", "grimoire/cmd/http", core, ""},
		{"domain root->module ok", core, core + "/internal/auth", ""},
		{"module root->base ok", core + "/internal/auth", core + "/internal/auth/base", ""},
		{"module root->feature ok", core + "/internal/auth", core + "/internal/auth/login", ""},
		{"feature->base ok", core + "/internal/auth/login", core + "/internal/auth/base", ""},
		{"feature->feature sama ok", core + "/internal/auth/login", core + "/internal/auth/login/repository", ""},
		{"base->feature (bukan 4.3)", core + "/internal/auth/base", core + "/internal/auth/login", ""},
		{"infra->infra ok", "grimoire/internal/infra/app", "grimoire/internal/infra/bus", ""},
		{"infra->lib ok", "grimoire/internal/infra/bus", "grimoire/lib/common/errs", ""},
		{"archtest->apa saja ok", "grimoire/internal/archtest", core, ""},
		{"contracts->contracts ok", "grimoire/lib/contracts/a", "grimoire/lib/contracts/b", ""},
		{"contracts->common ok", "grimoire/lib/contracts/a", "grimoire/lib/common/errs", ""},
		{"lib->lib ok", "grimoire/lib/util", "grimoire/lib/common/errs", ""},
		{"stdlib ok", "grimoire/lib/contracts/a", "context", ""},
		{"pihak ketiga ok", core + "/internal/auth", "github.com/jmoiron/sqlx", ""},
		{"prefix mirip bukan internal", "grimoire/lib/util", "grimoire-x/internal/infra", ""},
		{"self edge ok", core + "/internal/auth", core + "/internal/auth", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Check([]Import{{c.from, c.to}})
			if c.rule == "" {
				if len(got) != 0 {
					t.Fatalf("want none, got %v", got)
				}
				return
			}
			if len(got) != 1 || got[0].Rule != c.rule {
				t.Fatalf("want rule %s, got %v", c.rule, got)
			}
		})
	}
}

func TestCheck_DedupeAndSorted(t *testing.T) {
	in := []Import{
		{"grimoire/lib/util", "grimoire/internal/infra/bus"},
		{core + "/internal/a", project + "/internal/b"},
		{core + "/internal/a", project + "/internal/b"},
		{"grimoire/internal/infra/bus", core},
	}
	got := Check(in)
	if len(got) != 3 {
		t.Fatalf("want 3, got %v", got)
	}
	if got[0].Rule != Rule41 || got[1].Rule != Rule46 || got[2].Rule != Rule49 {
		t.Fatalf("order: %v", got)
	}
}

func TestViolation_String(t *testing.T) {
	s := Violation{Rule: "4.2", From: "a", To: "b", Reason: "r"}.String()
	if s != "rules.txt 4.2: a imports b (r)" {
		t.Fatal(s)
	}
}

func TestRule44_ReasonsMentionIt(t *testing.T) {
	for _, im := range []Import{
		{core + "/internal/a", project + "/internal/b"},
		{project + "/internal/a", project + "/internal/b"},
	} {
		v := Check([]Import{im})
		if len(v) != 1 || !strings.Contains(v[0].Reason, "rules.txt 4.4") {
			t.Fatalf("%v", v)
		}
	}
}

func TestT00_AC4_Rule42_ExactMessage(t *testing.T) {
	v := Check([]Import{{project + "/internal/knowledge/upload", project + "/internal/apikey/base"}})
	want := "rules.txt 4.2: grimoire/internal/domain/project/internal/knowledge/upload imports grimoire/internal/domain/project/internal/apikey/base (module -> module lain; gunakan infra/bus + lib/contracts, lihat rules.txt 4.4)"
	if len(v) != 1 || v[0].String() != want {
		t.Fatalf("got %v", v)
	}
}

func TestCheck_PrefixLookalikeIsExternal(t *testing.T) {
	for _, im := range []Import{
		{"grimoire-extra/internal/domain/x", "grimoire/internal/domain/core"},
		{"grimoire2/lib", "grimoire/internal/infra/bus"},
	} {
		if v := Check([]Import{im}); len(v) != 0 {
			t.Fatalf("%v: %v", im, v)
		}
	}
}

func TestCheck_OneViolationPerEdge(t *testing.T) {
	v := Check([]Import{{"grimoire/lib/contracts/core", core}})
	if len(v) != 1 || v[0].Rule != Rule45 {
		t.Fatalf("got %v", v)
	}
}

//go:build integration

package record

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"grimoire/internal/infra/app"
	"grimoire/internal/infra/bus"
	"grimoire/internal/infra/db"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/audit/record"
)

func setup(t *testing.T) *db.DB {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Migrate(context.Background(), d, os.DirFS("../../../../../../migrations/core"), slog.Default()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return d
}

func realUC(d *db.DB) *Usecase {
	return NewUsecase(NewRepo(d), time.Now, newUUIDv7)
}

func countRows(t *testing.T, d *db.DB) int {
	t.Helper()
	var n int
	if err := d.Get(&n, "SELECT count(*) FROM audit_log"); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestT02_AC1_FullAPIKeyEntryStored(t *testing.T) {
	d := setup(t)
	req := validReq()
	req.ActorLabel = "claude-code-budi (gk_ab12)"
	req.TraceID = "trace-1"
	at := time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)
	req.OccurredAt = at
	req.ID = fixedID
	if resp, err := realUC(d).Execute(context.Background(), req); err != nil || !resp.Recorded {
		t.Fatalf("resp=%v err=%v", resp, err)
	}
	var row struct {
		ID, ActorType, ActorID, ActorLabel, ProjectID, ProjectLabel, Action, TargetType, Target, Status, IPHost, IPType, UA, Trace, Path string
		Occurred                                                                                                                         time.Time
		Bytes                                                                                                                            int
	}
	err := d.QueryRow(`SELECT id::text, actor_type, actor_id::text, actor_label, project_id::text, project_label, action,
		target_type, target, status, host(ip), pg_typeof(ip)::text, user_agent, trace_id, args->>'path', occurred_at,
		(result->>'bytes')::int FROM audit_log`).Scan(&row.ID, &row.ActorType, &row.ActorID, &row.ActorLabel, &row.ProjectID,
		&row.ProjectLabel, &row.Action, &row.TargetType, &row.Target, &row.Status, &row.IPHost, &row.IPType, &row.UA,
		&row.Trace, &row.Path, &row.Occurred, &row.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if row.ID != fixedID || row.ActorType != "api_key" || row.ActorID != uidActor || row.ActorLabel != req.ActorLabel ||
		row.ProjectID != uidProject || row.ProjectLabel != "Payments" || row.Action != "mcp.read_file" ||
		row.TargetType != "file" || row.Target != "docs/auth/login.md" || row.Status != "ok" ||
		row.IPHost != "203.0.113.5" || row.IPType != "inet" || row.UA != "claude-code/2.1" || row.Trace != "trace-1" ||
		row.Path != "docs/auth/login.md" || row.Bytes != 4812 || !row.Occurred.Equal(at) {
		t.Fatalf("row: %+v", row)
	}
}

func TestT02_AC1_ViaBusCall(t *testing.T) {
	d := setup(t)
	reg := bus.NewRegistry(nil)
	Init(app.Deps{DB: d, Bus: reg})
	resp, err := bus.CallTyped[record.Response](context.Background(), reg, constants.BUS_CORE_AUDIT_RECORD, validReq())
	if err != nil || !resp.Recorded || countRows(t, d) != 1 {
		t.Fatalf("resp=%v err=%v", resp, err)
	}
}

func TestT02_AC2_AnonymousEntry(t *testing.T) {
	d := setup(t)
	req := record.Request{ActorType: record.ActorAnonymous, ActorLabel: "gk_zz99", Action: record.ActionMCPAuthRejected, Status: record.StatusDenied}
	if _, err := realUC(d).Execute(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	var a, p, pl sql.NullString
	var label string
	if err := d.QueryRow("SELECT actor_id::text, project_id::text, project_label, actor_label FROM audit_log").Scan(&a, &p, &pl, &label); err != nil {
		t.Fatal(err)
	}
	if a.Valid || p.Valid || pl.Valid || label != "gk_zz99" {
		t.Fatalf("a=%v p=%v pl=%v label=%q", a, p, pl, label)
	}
}

func TestT02_AC3_NoForeignKeys(t *testing.T) {
	d := setup(t)
	req := validReq()
	req.ActorID = "0192f0c4-aaaa-7000-8000-00000000000a"
	req.ProjectID = "0192f0c4-bbbb-7000-8000-00000000000b"
	if _, err := realUC(d).Execute(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := d.Get(&n, "SELECT count(*) FROM pg_constraint WHERE conrelid = 'audit_log'::regclass AND contype = 'f'"); err != nil || n != 0 {
		t.Fatalf("fk=%d err=%v", n, err)
	}
}

func TestT02_AC4_NoSecretsInJSONB(t *testing.T) {
	d := setup(t)
	req := validReq()
	req.Args = map[string]any{"password": "hunter2", "token": "tok-abc", "content": "ISI-FILE-RAHASIA", "path": "a.md"}
	req.Result = map[string]any{"content": "ISI-FILE-RAHASIA"}
	if _, err := realUC(d).Execute(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	var s, path string
	if err := d.QueryRow("SELECT args::text || result::text, args->>'path' FROM audit_log").Scan(&s, &path); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"hunter2", "tok-abc", "ISI-FILE-RAHASIA"} {
		if strings.Contains(s, bad) {
			t.Fatalf("leaked %s in %s", bad, s)
		}
	}
	if !strings.Contains(s, "[REDACTED]") || !strings.Contains(s, "[OMITTED]") || path != "a.md" {
		t.Fatalf("s=%s path=%s", s, path)
	}
}

func TestRecord_EmptyStringsBecomeNull(t *testing.T) {
	d := setup(t)
	req := validReq()
	req.TargetType, req.Target, req.UserAgent, req.IP = "", "", "", ""
	if _, err := realUC(d).Execute(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	var tt, tg, ua, ip sql.NullString
	var trace string
	if err := d.QueryRow("SELECT target_type, target, user_agent, ip::text, trace_id FROM audit_log").Scan(&tt, &tg, &ua, &ip, &trace); err != nil {
		t.Fatal(err)
	}
	if tt.Valid || tg.Valid || ua.Valid || ip.Valid || trace != "" {
		t.Fatalf("%v %v %v %v %q", tt, tg, ua, ip, trace)
	}
}

func TestRecord_IPFormats(t *testing.T) {
	d := setup(t)
	cases := map[string]string{"::1": "::1", "203.0.113.5:4431": "203.0.113.5", "not-an-ip": ""}
	for in, want := range cases {
		if _, err := d.Exec("DELETE FROM audit_log"); err != nil {
			t.Fatal(err)
		}
		req := validReq()
		req.IP = in
		if _, err := realUC(d).Execute(context.Background(), req); err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		var ip sql.NullString
		if err := d.QueryRow("SELECT host(ip) FROM audit_log").Scan(&ip); err != nil {
			t.Fatal(err)
		}
		if (want == "") == ip.Valid || (want != "" && ip.String != want) {
			t.Fatalf("%s: got %v want %q", in, ip, want)
		}
	}
}

func TestRecord_JSONBDefaultsToEmptyObject(t *testing.T) {
	d := setup(t)
	req := validReq()
	req.Args, req.Result = nil, nil
	if _, err := realUC(d).Execute(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	var a, r string
	if err := d.QueryRow("SELECT args::text, result::text FROM audit_log").Scan(&a, &r); err != nil || a != "{}" || r != "{}" {
		t.Fatalf("a=%s r=%s err=%v", a, r, err)
	}
}

func TestRecord_DuplicateIDError(t *testing.T) {
	d := setup(t)
	req := validReq()
	req.ID = fixedID
	uc := realUC(d)
	if _, err := uc.Execute(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	resp, err := uc.Execute(context.Background(), req)
	if err == nil || resp.Recorded || !strings.HasPrefix(err.Error(), "audit record: ") || countRows(t, d) != 1 {
		t.Fatalf("resp=%v err=%v", resp, err)
	}
}

func TestRecord_ContextCanceled(t *testing.T) {
	d := setup(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	resp, err := realUC(d).Execute(ctx, validReq())
	if err == nil || resp.Recorded || !errors.Is(err, context.Canceled) || countRows(t, d) != 0 {
		t.Fatalf("resp=%v err=%v", resp, err)
	}
}

func TestRecord_SQLParameterized(t *testing.T) {
	d := setup(t)
	evil := "'); DROP TABLE audit_log;--"
	req := validReq()
	req.Target, req.ActorLabel = evil, evil
	if _, err := realUC(d).Execute(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	var target string
	if err := d.Get(&target, "SELECT target FROM audit_log"); err != nil || target != evil {
		t.Fatalf("target=%q err=%v", target, err)
	}
}

//go:build integration

package createadmin

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/bus"
	"grimoire/internal/infra/db"
	"grimoire/lib/common/errs"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/audit/record"
	"grimoire/lib/contracts/core/auth/createadmin"
	"grimoire/migrations"
)

func integEnv(t *testing.T, withAudit bool) (*bus.Registry, *db.DB, base.Hasher, *bytes.Buffer, *[]record.Request) {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Migrate(context.Background(), d, migrations.FS, nil); err != nil {
		t.Fatal(err)
	}
	buf := &bytes.Buffer{}
	log := slog.New(slog.NewTextHandler(buf, nil))
	b := bus.NewRegistry(log)
	var mu sync.Mutex
	got := &[]record.Request{}
	if withAudit {
		b.Register(constants.BUS_CORE_AUDIT_RECORD, func(_ context.Context, req any) (any, error) {
			mu.Lock()
			defer mu.Unlock()
			*got = append(*got, req.(record.Request))
			return record.Response{}, nil
		})
	}
	h := base.Argon2Hasher{Params: base.Argon2Params{MemoryKiB: 64, Iterations: 1, Parallelism: 1}, Rand: rand.Reader}
	Init(app.Deps{DB: d, Bus: b, Log: log}, base.Shared{Hasher: h, Password: base.PasswordPolicy{MinLength: 10}})
	return b, d, h, buf, got
}

func countUsers(t *testing.T, d *db.DB) int {
	t.Helper()
	var n int
	if err := d.QueryRowContext(context.Background(), "SELECT count(*) FROM users").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestCreateAdmin_PersistsRow(t *testing.T) {
	b, d, h, _, _ := integEnv(t, true)
	ctx := context.Background()
	res, err := bus.CallTyped[createadmin.Response](ctx, b, constants.BUS_CORE_AUTH_CREATE_ADMIN, validReq())
	if err != nil {
		t.Fatal(err)
	}
	var role, hash, label string
	var mcp, active bool
	var createdBy *string
	err = d.QueryRowContext(ctx, "SELECT role, password_hash, must_change_password, is_active, created_by::text, created_by_label FROM users WHERE id=$1", res.UserID).
		Scan(&role, &hash, &mcp, &active, &createdBy, &label)
	if err != nil {
		t.Fatal(err)
	}
	if role != "super_admin" || mcp || !active || createdBy != nil || label != "cli" {
		t.Errorf("row = %s %v %v %v %s", role, mcp, active, createdBy, label)
	}
	if ok, err := h.Verify(hash, "kata-sandi-panjang"); err != nil || !ok {
		t.Errorf("verify = %v %v", ok, err)
	}
}

func TestT01_AC1_DuplicateEmailCaseInsensitive(t *testing.T) {
	b, d, _, _, _ := integEnv(t, true)
	ctx := context.Background()
	if _, err := b.Call(ctx, constants.BUS_CORE_AUTH_CREATE_ADMIN, validReq()); err != nil {
		t.Fatal(err)
	}
	req := validReq()
	req.Email = "ADMIN@TIM.ID"
	_, err := b.Call(ctx, constants.BUS_CORE_AUTH_CREATE_ADMIN, req)
	var ee *errs.Error
	if !errors.As(err, &ee) || ee.Code != errs.CodeConflict {
		t.Fatalf("err = %v", err)
	}
	if n := countUsers(t, d); n != 1 {
		t.Errorf("users = %d", n)
	}
}

func TestCreateAdmin_ConcurrentSameEmail(t *testing.T) {
	b, d, _, _, _ := integEnv(t, true)
	var ok, conflict atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := b.Call(context.Background(), constants.BUS_CORE_AUTH_CREATE_ADMIN, validReq())
			var ee *errs.Error
			switch {
			case err == nil:
				ok.Add(1)
			case errors.As(err, &ee) && ee.Code == errs.CodeConflict:
				conflict.Add(1)
			default:
				t.Errorf("err = %v", err)
			}
		}()
	}
	wg.Wait()
	if ok.Load() != 1 || conflict.Load() != 7 || countUsers(t, d) != 1 {
		t.Errorf("ok=%d conflict=%d", ok.Load(), conflict.Load())
	}
}

func TestCreateAdmin_AuditRecordedViaBus(t *testing.T) {
	b, _, _, _, got := integEnv(t, true)
	ctx := context.Background()
	res, err := bus.CallTyped[createadmin.Response](ctx, b, constants.BUS_CORE_AUTH_CREATE_ADMIN, validReq())
	if err != nil {
		t.Fatal(err)
	}
	if len(*got) != 1 {
		t.Fatalf("audit = %d", len(*got))
	}
	r := (*got)[0]
	if r.ActorType != "system" || r.ActorLabel != "cli" || r.Action != "user.created" || r.Target != res.UserID {
		t.Errorf("record = %+v", r)
	}
	_, _ = b.Call(ctx, constants.BUS_CORE_AUTH_CREATE_ADMIN, validReq())
	if len(*got) != 1 {
		t.Errorf("audit pada CONFLICT = %d", len(*got))
	}
}

func TestCreateAdmin_AuditUnregistered_StillSucceeds(t *testing.T) {
	b, d, _, buf, _ := integEnv(t, false)
	if _, err := b.Call(context.Background(), constants.BUS_CORE_AUTH_CREATE_ADMIN, validReq()); err != nil {
		t.Fatal(err)
	}
	if countUsers(t, d) != 1 || !bytes.Contains(buf.Bytes(), []byte("audit record gagal")) {
		t.Errorf("log = %s", buf)
	}
}

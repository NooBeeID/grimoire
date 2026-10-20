//go:build integration

package getuserbus

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/domain/core/internal/auth/base/store"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/bus"
	"grimoire/internal/infra/db"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/auth/getuser"
	"grimoire/migrations"
)

func setupInt(t *testing.T) (*bus.Registry, *store.Store) {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Migrate(context.Background(), d, migrations.FS, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	r := bus.NewRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	Init(app.Deps{DB: d, Bus: r}, base.Shared{})
	return r, store.New(d)
}

func call(r *bus.Registry, id string) (getuser.Response, error) {
	return bus.CallTyped[getuser.Response](context.Background(), r, constants.BUS_CORE_AUTH_GET_USER, getuser.Request{UserID: id})
}

func insert(t *testing.T, s *store.Store, email string, active bool) base.User {
	t.Helper()
	u, err := s.InsertUser(context.Background(), base.User{Email: email, Name: "N", PasswordHash: "$argon2id$secret", Role: base.RoleDeveloper, IsActive: active})
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestGetUserBus_Integration_Found(t *testing.T) {
	r, s := setupInt(t)
	u := insert(t, s, "a@x.id", true)
	res, err := call(r, u.ID)
	want := getuser.Response{Found: true, ID: u.ID, Name: u.Name, Email: u.Email, Role: u.Role, IsActive: true}
	if err != nil || res != want {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestGetUserBus_Integration_Inactive(t *testing.T) {
	r, s := setupInt(t)
	u := insert(t, s, "b@x.id", false)
	res, err := call(r, u.ID)
	if err != nil || !res.Found || res.IsActive {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestGetUserBus_Integration_UnknownUUID(t *testing.T) {
	r, _ := setupInt(t)
	res, err := call(r, "00000000-0000-4000-8000-000000000000")
	if err != nil || res.Found {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestGetUserBus_Integration_InvalidUUID(t *testing.T) {
	r, s := setupInt(t)
	insert(t, s, "c@x.id", true)
	for _, id := range []string{"abc", "123", "'; DROP TABLE users;--", "", strings.Repeat("a", 1000)} {
		res, err := call(r, id)
		if err != nil || res.Found {
			t.Fatalf("%q: %+v %v", id, res, err)
		}
	}
	if res, err := call(r, "00000000-0000-4000-8000-000000000000"); err != nil || res.Found {
		t.Fatalf("users table check: %+v %v", res, err)
	}
}

func TestGetUserBus_Integration_NoSensitiveFields(t *testing.T) {
	r, s := setupInt(t)
	u := insert(t, s, "d@x.id", true)
	res, _ := call(r, u.ID)
	if n := reflect.TypeOf(res).NumField(); n != 6 {
		t.Fatalf("fields = %d", n)
	}
	if strings.Contains(fmt.Sprintf("%+v", res), "$argon2id$") {
		t.Fatal("hash leaked")
	}
}

func TestGetUserBus_Integration_Concurrent(t *testing.T) {
	r, s := setupInt(t)
	u := insert(t, s, "e@x.id", true)
	var wg sync.WaitGroup
	var bad atomic.Int32
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := call(r, u.ID)
			if err != nil || !res.Found || res.ID != u.ID {
				bad.Add(1)
			}
		}()
	}
	wg.Wait()
	if bad.Load() != 0 {
		t.Fatalf("%d inconsistent", bad.Load())
	}
}

func TestGetUserBus_Integration_NoAudit(t *testing.T) {
	r, s := setupInt(t)
	var n atomic.Int32
	r.Register(constants.BUS_CORE_AUDIT_RECORD, func(context.Context, any) (any, error) { n.Add(1); return nil, nil })
	u := insert(t, s, "f@x.id", true)
	_, _ = call(r, u.ID)
	_, _ = call(r, "bukan-uuid")
	if n.Load() != 0 {
		t.Fatalf("audit calls = %d", n.Load())
	}
}

//go:build integration

package finddevelopers

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"grimoire/internal/infra/bus"
	"grimoire/internal/infra/db"
	"grimoire/lib/constants"
	contract "grimoire/lib/contracts/core/auth/finddevelopers"
	"grimoire/migrations"
)

func setupInt(t *testing.T) (*db.DB, *bus.Registry) {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Migrate(context.Background(), d, migrations.FS, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	reg := bus.NewRegistry(nil)
	reg.Register(constants.BUS_CORE_AUTH_FIND_DEVELOPERS, NewHandler(NewUsecase(NewRepository(d))).Handle)
	return d, reg
}

func seed(t *testing.T, d *db.DB, name, email, role string, active bool) {
	t.Helper()
	_, err := d.Exec(`INSERT INTO users (email, name, password_hash, role, is_active, must_change_password) VALUES ($1,$2,'h',$3,$4,false)`, email, name, role, active)
	if err != nil {
		t.Fatalf("seed %s: %v", email, err)
	}
}

func find(t *testing.T, reg *bus.Registry, q string, limit int) contract.Response {
	t.Helper()
	res, err := bus.CallTyped[contract.Response](context.Background(), reg, constants.BUS_CORE_AUTH_FIND_DEVELOPERS, contract.Request{Query: q, Limit: limit})
	if err != nil {
		t.Fatalf("find %q: %v", q, err)
	}
	if res.Items == nil {
		t.Fatalf("find %q: nil Items", q)
	}
	return res
}

func names(r contract.Response) []string {
	var out []string
	for _, i := range r.Items {
		out = append(out, i.Name)
	}
	return out
}

func seedBase(t *testing.T, d *db.DB) {
	seed(t, d, "Sari Wulandari", "sari@tim.id", "developer", true)
	seed(t, d, "Andi Saputra", "andi@tim.id", "developer", true)
	seed(t, d, "Budi Santoso", "budi@tim.id", "developer", true)
	seed(t, d, "Sari Nonaktif", "sari.off@tim.id", "developer", false)
	seed(t, d, "Sari Admin", "sari.admin@tim.id", "super_admin", true)
	seed(t, d, "50% Diskon", "diskon_50@tim.id", "developer", true)
	seed(t, d, "Zed", "zed@tim.id", "developer", true)
}

func TestFindDevelopers_Integration_Filtering(t *testing.T) {
	d, reg := setupInt(t)
	seedBase(t, d)
	if n := names(find(t, reg, "sari", 0)); len(n) != 1 || n[0] != "Sari Wulandari" {
		t.Fatalf("only active developers: %v", n)
	}
	if n := names(find(t, reg, "andi", 0)); len(n) != 1 {
		t.Fatalf("andi: %v", n)
	}
	if n := names(find(t, reg, "SANTOSO", 0)); len(n) != 1 || n[0] != "Budi Santoso" {
		t.Fatalf("case-insens: %v", n)
	}
	all := names(find(t, reg, "@tim.id", 0))
	want := []string{"50% Diskon", "Andi Saputra", "Budi Santoso", "Sari Wulandari", "Zed"}
	if strings.Join(all, ",") != strings.Join(want, ",") {
		t.Fatalf("order: %v", all)
	}
	if n := find(t, reg, "tidak-ada-xyz", 0); len(n.Items) != 0 {
		t.Fatalf("no match: %v", n)
	}
}

func TestFindDevelopers_Integration_OrderTieByID(t *testing.T) {
	d, reg := setupInt(t)
	seed(t, d, "Kembar", "k1@tim.id", "developer", true)
	seed(t, d, "Kembar", "k2@tim.id", "developer", true)
	r := find(t, reg, "kembar", 0)
	if len(r.Items) != 2 || r.Items[0].ID >= r.Items[1].ID {
		t.Fatalf("%+v", r.Items)
	}
}

func TestFindDevelopers_Integration_Limits(t *testing.T) {
	d, reg := setupInt(t)
	for i := 1; i <= 60; i++ {
		seed(t, d, fmt.Sprintf("Dev %02d", i), fmt.Sprintf("dev%02d@tim.id", i), "developer", true)
	}
	if n := len(find(t, reg, "dev", 0).Items); n != 10 {
		t.Fatalf("default: %d", n)
	}
	if n := len(find(t, reg, "dev", 1000).Items); n != 50 {
		t.Fatalf("clamp: %d", n)
	}
	if n := len(find(t, reg, "dev", 3).Items); n != 3 {
		t.Fatalf("3: %d", n)
	}
}

func TestFindDevelopers_Integration_EmptyQueryNoDB(t *testing.T) {
	d, reg := setupInt(t)
	seedBase(t, d)
	for _, q := range []string{"", "  "} {
		if r := find(t, reg, q, 0); len(r.Items) != 0 {
			t.Fatalf("%q: %v", q, r)
		}
	}
	d.Close() // DB tertutup: query kosong tetap sukses karena tidak query
	if r := find(t, reg, "", 0); len(r.Items) != 0 {
		t.Fatal("expected empty")
	}
	if _, err := reg.Call(context.Background(), constants.BUS_CORE_AUTH_FIND_DEVELOPERS, contract.Request{Query: "sari"}); err == nil {
		t.Fatal("DB error must surface, not empty list")
	}
}

func TestFindDevelopers_Integration_WildcardAndInjection(t *testing.T) {
	d, reg := setupInt(t)
	seedBase(t, d)
	if n := names(find(t, reg, "%", 0)); len(n) != 1 || n[0] != "50% Diskon" {
		t.Fatalf("%%: %v", n)
	}
	if n := names(find(t, reg, "_", 0)); len(n) != 1 || n[0] != "50% Diskon" {
		t.Fatalf("_: %v", n)
	}
	if n := names(find(t, reg, `\`, 0)); len(n) != 0 {
		t.Fatalf("backslash: %v", n)
	}
	if n := names(find(t, reg, "'; DROP TABLE users;--", 0)); len(n) != 0 {
		t.Fatalf("injection: %v", n)
	}
	var cnt int
	if err := d.Get(&cnt, `SELECT count(*) FROM users`); err != nil || cnt != 7 {
		t.Fatalf("users: %d %v", cnt, err)
	}
	if n := find(t, reg, strings.Repeat("x", 10000), 0); len(n.Items) != 0 {
		t.Fatal("long query")
	}
}

func TestFindDevelopers_Integration_StateChangesImmediate(t *testing.T) {
	d, reg := setupInt(t)
	seedBase(t, d)
	if len(find(t, reg, "andi", 0).Items) != 1 {
		t.Fatal("precondition")
	}
	d.MustExec(`UPDATE users SET is_active=false WHERE email='andi@tim.id'`)
	if len(find(t, reg, "andi", 0).Items) != 0 {
		t.Fatal("deactivated still found")
	}
	d.MustExec(`UPDATE users SET role='super_admin' WHERE email='budi@tim.id'`)
	if len(find(t, reg, "budi", 0).Items) != 0 {
		t.Fatal("role change still found")
	}
}

func TestFindDevelopers_Integration_NoSensitiveFields(t *testing.T) {
	d, reg := setupInt(t)
	seedBase(t, d)
	r := find(t, reg, "sari", 0)
	if s := fmt.Sprintf("%+v", r); strings.Contains(s, "argon2id") {
		t.Fatal(s)
	}
}

func TestFindDevelopers_Integration_Concurrent(t *testing.T) {
	d, reg := setupInt(t)
	seedBase(t, d)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if r := find(t, reg, "sari", 0); len(r.Items) != 1 {
				t.Errorf("%v", r)
			}
		}()
	}
	wg.Wait()
}

func seedInvitation(t *testing.T, d *db.DB, email string, expires string, accepted bool) {
	t.Helper()
	acc := "NULL"
	if accepted {
		acc = "now()"
	}
	_, err := d.Exec(`INSERT INTO user_invitations (user_id, invited_by, issued_at, expires_at, accepted_at)
SELECT id, id, now(), `+expires+`, `+acc+` FROM users WHERE email = $1`, email)
	if err != nil {
		t.Fatalf("seed invitation %s: %v", email, err)
	}
}

func TestFindDevelopers_Integration_InvitationPending(t *testing.T) {
	d, reg := setupInt(t)
	seed(t, d, "Inv Menunggu", "inv-a@tim.id", "developer", true)
	seed(t, d, "Inv Diterima", "inv-b@tim.id", "developer", true)
	seed(t, d, "Inv Biasa", "inv-c@tim.id", "developer", true)
	seed(t, d, "Inv Kedaluwarsa", "inv-d@tim.id", "developer", true)
	seed(t, d, "Inv Nonaktif", "inv-e@tim.id", "developer", false)
	seedInvitation(t, d, "inv-a@tim.id", "now() + interval '1 day'", false)
	seedInvitation(t, d, "inv-b@tim.id", "now() + interval '1 day'", true)
	seedInvitation(t, d, "inv-d@tim.id", "now() - interval '1 day'", false)
	seedInvitation(t, d, "inv-e@tim.id", "now() + interval '1 day'", false)

	res := find(t, reg, "inv-", 0)
	got := map[string]bool{}
	for _, it := range res.Items {
		got[it.Email] = it.InvitationPending
	}
	want := map[string]bool{"inv-a@tim.id": true, "inv-b@tim.id": false, "inv-c@tim.id": false, "inv-d@tim.id": true}
	if len(res.Items) != 4 || fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("got %v items=%d", got, len(res.Items))
	}
	// urutan nama ASC tanpa baris ganda
	if n := names(res); fmt.Sprint(n) != "[Inv Biasa Inv Diterima Inv Kedaluwarsa Inv Menunggu]" {
		t.Fatalf("%v", n)
	}
	if lim := find(t, reg, "inv-", 2); len(lim.Items) != 2 {
		t.Fatalf("limit: %d", len(lim.Items))
	}
}

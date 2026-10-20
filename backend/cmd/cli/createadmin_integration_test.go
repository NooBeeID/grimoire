//go:build integration

package main

import (
	"database/sql"
	"regexp"
	"strings"
	"testing"
)

func TestT01_AC1_CreateAdminEndToEnd(t *testing.T) {
	dbURL := testSchemaURL(t)
	env := func(pw string) []string {
		return []string{"DATABASE_URL=" + dbURL, "SESSION_SECRET=" + secret, "CREATE_ADMIN_PASSWORD=" + pw}
	}
	const pw = "kata-sandi-panjang"
	args := []string{"create-admin", "--email", "admin@tim.id", "--name", "Admin"}

	// Sebelum migrate up: pesan petunjuk.
	_, errOut, code := runBin(t, env(pw), "", args...)
	if code != 1 || !strings.Contains(errOut, "grimoire migrate up") {
		t.Fatalf("sebelum migrate: code=%d stderr=%q", code, errOut)
	}
	if _, _, code := runBin(t, env(""), "", "migrate", "up"); code != 0 {
		t.Fatal("migrate up gagal")
	}

	out, errOut, code := runBin(t, env(pw), "", args...)
	if code != 0 || !regexp.MustCompile(`^admin admin@tim\.id dibuat \(id [0-9a-f-]{36}\)\n$`).MatchString(out) {
		t.Fatalf("sukses: code=%d out=%q stderr=%q", code, out, errOut)
	}
	if strings.Contains(out+errOut, pw) || strings.Contains(out+errOut, "$argon2id$") {
		t.Error("password/hash bocor")
	}

	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var role, label string
	var mcp, active bool
	if err := db.QueryRow(`SELECT role, must_change_password, is_active, created_by_label FROM users WHERE email='admin@tim.id'`).
		Scan(&role, &mcp, &active, &label); err != nil {
		t.Fatal(err)
	}
	if role != "super_admin" || mcp || !active || label != "cli" {
		t.Errorf("row = %s %v %v %s", role, mcp, active, label)
	}

	// Email sama (beda huruf besar): CONFLICT.
	_, errOut, code = runBin(t, env(pw), "", "create-admin", "--email", "ADMIN@tim.id", "--name", "Admin")
	if code != 1 || !strings.Contains(errOut, "email sudah terdaftar") || strings.Contains(errOut, pw) {
		t.Fatalf("duplikat: code=%d stderr=%q", code, errOut)
	}
	// Password pendek.
	_, errOut, code = runBin(t, env("pendek"), "", "create-admin", "--email", "b@tim.id", "--name", "B")
	if code != 1 || !strings.Contains(errOut, "password: minimal 10 karakter") {
		t.Fatalf("validasi: code=%d stderr=%q", code, errOut)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM users`).Scan(&n); err != nil || n != 1 {
		t.Errorf("users = %d err=%v", n, err)
	}
}

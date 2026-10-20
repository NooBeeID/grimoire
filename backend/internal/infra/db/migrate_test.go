package db

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	"grimoire/internal/infra/config"
	"grimoire/migrations"
)

func files(names ...string) fstest.MapFS {
	m := fstest.MapFS{}
	for _, n := range names {
		m[n] = &fstest.MapFile{Data: []byte("SELECT 1;")}
	}
	return m
}

func TestPlan_ValidOrderAcrossFolders(t *testing.T) {
	list, err := plan(files(
		"project/20261020080000002_b.sql",
		"core/20261020080000001_a.sql",
		"core/20261020080000003_c.sql",
	))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, m := range list {
		got = append(got, m.Name)
	}
	want := "20261020080000001_a.sql,20261020080000002_b.sql,20261020080000003_c.sql"
	if strings.Join(got, ",") != want {
		t.Fatalf("got %v", got)
	}
}

func TestPlan_IgnoresNonSQL(t *testing.T) {
	list, err := plan(files("core/.keep", "project/.keep", "core/README.md", "core/x.sql.bak", "core/Y.SQL"))
	if err != nil || len(list) != 0 {
		t.Fatalf("list=%v err=%v", list, err)
	}
}

func TestPlan_InvalidNamesRejected(t *testing.T) {
	for _, n := range []string{
		"2026102008000000_x.sql",
		"202610200800000000_x.sql",
		"20261020080000000.sql",
		"20261020080000000_.sql",
		"20261020080000000_Create.sql",
		"20261020080000000_create-users.sql",
		"20261020080000000_a b.sql",
		"v20261020080000000_x.sql",
	} {
		path := "core/" + n
		_, err := plan(files(path))
		if !errors.Is(err, ErrInvalidMigrationName) {
			t.Errorf("%s: err=%v", n, err)
			continue
		}
		if !strings.Contains(err.Error(), path) {
			t.Errorf("%s: pesan tanpa path: %v", n, err)
		}
	}
}

func TestPlan_DuplicateVersionRejected(t *testing.T) {
	_, err := plan(files("core/20261020080000000_a.sql", "project/20261020080000000_b.sql"))
	if !errors.Is(err, ErrDuplicateVersion) {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(err.Error(), "core/20261020080000000_a.sql") || !strings.Contains(err.Error(), "project/20261020080000000_b.sql") {
		t.Fatalf("pesan: %v", err)
	}
}

func TestPlan_DuplicateVersionSameFolder(t *testing.T) {
	_, err := plan(files("core/20261020080000000_a.sql", "core/20261020080000000_b.sql"))
	if !errors.Is(err, ErrDuplicateVersion) {
		t.Fatalf("err=%v", err)
	}
}

func TestPlan_ErrorBeforeAnyExecution(t *testing.T) {
	_, err := Migrate(context.Background(), nil, files("core/bad.sql"), nil)
	if !errors.Is(err, ErrInvalidMigrationName) {
		t.Fatalf("err=%v", err)
	}
	_, err = Migrate(context.Background(), nil, files("core/20261020080000000_a.sql", "project/20261020080000000_b.sql"), nil)
	if !errors.Is(err, ErrDuplicateVersion) {
		t.Fatalf("err=%v", err)
	}
}

func TestPlan_EmbeddedFSValid(t *testing.T) {
	if _, err := plan(migrations.FS); err != nil {
		t.Fatal(err)
	}
}

func TestPlan_DeterministicOrder(t *testing.T) {
	fsys := files("project/20261020080000002_b.sql", "core/20261020080000001_a.sql", "core/20261020080000003_c.sql")
	a, _ := plan(fsys)
	b, _ := plan(fsys)
	if len(a) != len(b) {
		t.Fatal("beda panjang")
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("beda di %d", i)
		}
	}
}

func TestOpen_ParseErrorDoesNotLeakPassword(t *testing.T) {
	_, err := Open(context.Background(), config.Config{DatabaseURL: "postgres://u:s3cretpw@%zz/db"})
	if err == nil {
		t.Fatal("harus error")
	}
	if strings.Contains(err.Error(), "s3cretpw") {
		t.Fatalf("password bocor: %v", err)
	}
}

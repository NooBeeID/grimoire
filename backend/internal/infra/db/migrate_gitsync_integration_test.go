//go:build integration

package db

import (
	"context"
	"testing"

	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// constraintOf mengembalikan nama constraint dari error Postgres (kosong bila bukan PgError).
func constraintOf(err error) string {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		return pg.ConstraintName
	}
	return ""
}

func migratedDB(t *testing.T) *DB {
	t.Helper()
	d := TestDB(t)
	if _, err := Up(context.Background(), d, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return d
}

func insUser(t *testing.T, d *DB, id, email string) {
	t.Helper()
	if _, err := d.Exec(`INSERT INTO users (id,email,name,password_hash,role) VALUES ($1,$2,'U','x','developer')`, id, email); err != nil {
		t.Fatalf("user: %v", err)
	}
}

const (
	gcUserA = "0192f5a0-aaaa-7aaa-8bbb-00000000000a"
	gcUserB = "0192f5a0-bbbb-7aaa-8bbb-00000000000b"
)

func insCred(d *DB, user, provider, host string, tokenBytes int) error {
	_, err := d.Exec(`INSERT INTO git_credentials (user_id,provider,host,token_enc,last4)
VALUES ($1,$2,$3,decode(repeat('ab',$4),'hex'),'7Qx2')`, user, provider, host, tokenBytes)
	return err
}

func TestGitCredentials_Constraints(t *testing.T) {
	d := migratedDB(t)
	insUser(t, d, gcUserA, "a@x.id")
	insUser(t, d, gcUserB, "b@x.id")

	cases := []struct {
		name       string
		provider   string
		host       string
		tokenBytes int
		want       string
	}{
		{"github host selain github.com ditolak", "github", "ghe.example.com", 40, "git_credentials_github_host"},
		{"token_enc 28 byte ditolak", "gitlab", "gitlab.com", 28, "git_credentials_token_enc_len"},
		{"token_enc kosong ditolak", "gitlab", "gitlab.com", 0, "git_credentials_token_enc_len"},
		{"provider tak dikenal ditolak", "bitbucket", "bitbucket.org", 40, "git_credentials_provider_check"},
		{"host huruf besar ditolak", "gitlab", "GitLab.com", 40, "git_credentials_host_check"},
	}
	for _, c := range cases {
		err := insCred(d, gcUserA, c.provider, c.host, c.tokenBytes)
		if err == nil || constraintOf(err) != c.want {
			t.Errorf("%s: err=%v constraint=%q want %q", c.name, err, constraintOf(err), c.want)
		}
	}
	// 29 byte (> 28) diterima; github.com diterima.
	if err := insCred(d, gcUserA, "gitlab", "gitlab.com", 29); err != nil {
		t.Fatalf("29 byte harus diterima: %v", err)
	}
	if err := insCred(d, gcUserA, "github", "github.com", 40); err != nil {
		t.Fatalf("github.com harus diterima: %v", err)
	}
}

func TestGitCredentials_UniquePerUserProviderHost(t *testing.T) {
	d := migratedDB(t)
	insUser(t, d, gcUserA, "a@x.id")
	insUser(t, d, gcUserB, "b@x.id")
	if err := insCred(d, gcUserA, "gitlab", "gitlab.com", 40); err != nil {
		t.Fatal(err)
	}
	err := insCred(d, gcUserA, "gitlab", "gitlab.com", 40)
	if constraintOf(err) != "git_credentials_user_provider_host_key" {
		t.Fatalf("duplikat harus ditolak unique: %v", err)
	}
	// host lain, provider lain, dan user lain tetap boleh.
	for _, c := range [][3]string{{gcUserA, "gitlab", "git.noobee.id"}, {gcUserA, "github", "github.com"}, {gcUserB, "gitlab", "gitlab.com"}} {
		if err := insCred(d, c[0], c[1], c[2], 40); err != nil {
			t.Fatalf("%v harus diterima: %v", c, err)
		}
	}
}

func TestGitCredentials_CascadeOnUserDelete(t *testing.T) {
	d := migratedDB(t)
	insUser(t, d, gcUserA, "a@x.id")
	insUser(t, d, gcUserB, "b@x.id")
	for _, u := range []string{gcUserA, gcUserB} {
		if err := insCred(d, u, "gitlab", "gitlab.com", 40); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := d.Exec(`DELETE FROM users WHERE id=$1`, gcUserA); err != nil {
		t.Fatal(err)
	}
	var a, b int
	_ = d.Get(&a, `SELECT count(*) FROM git_credentials WHERE user_id=$1`, gcUserA)
	_ = d.Get(&b, `SELECT count(*) FROM git_credentials WHERE user_id=$1`, gcUserB)
	if a != 0 || b != 1 {
		t.Fatalf("cascade: A=%d (want 0) B=%d (want 1)", a, b)
	}
}

func insProject(t *testing.T, d *DB, name string) string {
	t.Helper()
	var id string
	if err := d.Get(&id, `INSERT INTO projects (name) VALUES ($1) RETURNING id::text`, name); err != nil {
		t.Fatal(err)
	}
	return id
}

func insSync(d *DB, project, kind, prefix string) error {
	_, err := d.Exec(`INSERT INTO git_sync_configs (project_id,kind,provider,host,repo_url,repo_path,branch,target_prefix)
VALUES ($1,$2,'github','github.com','https://github.com/a/b','a/b','main',$3)`, project, kind, prefix)
	return err
}

func TestGitSyncConfigs_Constraints(t *testing.T) {
	d := migratedDB(t)
	p := insProject(t, d, "proj-a")

	// target_prefix hanya untuk knowledge.
	if err := insSync(d, p, "api_spec", "guides"); constraintOf(err) != "git_sync_configs_prefix_kind" {
		t.Fatalf("prefix api_spec harus ditolak: %v", err)
	}
	if err := insSync(d, p, "wiki", ""); constraintOf(err) != "git_sync_configs_kind_check" {
		t.Fatalf("kind tak dikenal harus ditolak: %v", err)
	}
	_, err := d.Exec(`INSERT INTO git_sync_configs (project_id,kind,provider,host,repo_url,repo_path,branch,last_status)
VALUES ($1,'knowledge','github','github.com','u','a/b','main','bogus')`, p)
	if constraintOf(err) != "git_sync_configs_status_check" {
		t.Fatalf("last_status tak dikenal harus ditolak: %v", err)
	}
	if err := insSync(d, p, "knowledge", "guides"); err != nil {
		t.Fatalf("prefix knowledge diterima: %v", err)
	}
	if err := insSync(d, p, "api_spec", ""); err != nil {
		t.Fatalf("api_spec tanpa prefix diterima: %v", err)
	}
}

func TestGitSyncConfigs_UniqueProjectKind(t *testing.T) {
	d := migratedDB(t)
	p := insProject(t, d, "proj-a")
	q := insProject(t, d, "proj-b")
	if err := insSync(d, p, "knowledge", ""); err != nil {
		t.Fatal(err)
	}
	if err := insSync(d, p, "knowledge", ""); constraintOf(err) != "git_sync_configs_project_kind_key" {
		t.Fatalf("duplikat (project,kind) harus ditolak: %v", err)
	}
	if err := insSync(d, p, "api_spec", ""); err != nil {
		t.Fatalf("kind lain boleh: %v", err)
	}
	if err := insSync(d, q, "knowledge", ""); err != nil {
		t.Fatalf("project lain boleh: %v", err)
	}
}

func TestGitSyncConfigs_CascadeOnProjectDelete(t *testing.T) {
	d := migratedDB(t)
	p := insProject(t, d, "proj-a")
	q := insProject(t, d, "proj-b")
	for _, pid := range []string{p, q} {
		if err := insSync(d, pid, "knowledge", ""); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := d.Exec(`DELETE FROM projects WHERE id=$1`, p); err != nil {
		t.Fatal(err)
	}
	var a, b int
	_ = d.Get(&a, `SELECT count(*) FROM git_sync_configs WHERE project_id=$1`, p)
	_ = d.Get(&b, `SELECT count(*) FROM git_sync_configs WHERE project_id=$1`, q)
	if a != 0 || b != 1 {
		t.Fatalf("cascade: p=%d (want 0) q=%d (want 1)", a, b)
	}
}

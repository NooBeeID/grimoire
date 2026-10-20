//go:build integration

package createuser

import (
	"context"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/domain/core/internal/auth/base/store"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/db"
	"grimoire/lib/common/reqctx"
	"grimoire/migrations"
)

func integRouter(t *testing.T) (http.Handler, *store.Store, base.Shared) {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Migrate(context.Background(), d, migrations.FS, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	s := store.New(d)
	admin, err := s.InsertUser(context.Background(), base.User{Email: "admin@tim.id", Name: "Admin",
		PasswordHash: "x", Role: base.RoleSuperAdmin, IsActive: true})
	if err != nil {
		t.Fatal(err)
	}
	api := chi.NewRouter()
	api.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(reqctx.WithIdentity(r.Context(),
				reqctx.Identity{UserID: admin.ID, Email: admin.Email, Role: base.RoleSuperAdmin})))
		})
	})
	sh := base.Shared{
		Hasher: base.Argon2Hasher{Params: base.Argon2Params{MemoryKiB: 64, Iterations: 1, Parallelism: 1}, Rand: rand.Reader},
		Clock:  base.SystemClock, Rand: rand.Reader, Password: base.PasswordPolicy{MinLength: 10},
	}
	Init(app.Deps{DB: d, API: api}, sh)
	root := chi.NewRouter()
	root.Mount("/api", api)
	return root, s, sh
}

func post(h http.Handler, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/users", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	return w
}

func TestCreateUser_PersistsRow(t *testing.T) {
	h, s, sh := integRouter(t)
	w := post(h, `{"email":" Sari@Tim.ID ","name":"Sari","role":"developer"}`)
	if w.Code != 201 {
		t.Fatalf("code=%d body=%s", w.Code, w.Body)
	}
	u, ok, err := s.FindUserByEmail(context.Background(), "sari@tim.id")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if !u.IsActive || !u.MustChangePassword || u.Role != "developer" || u.LastLoginAt != nil ||
		u.CreatedBy == nil || !strings.HasPrefix(u.PasswordHash, "$argon2id$") {
		t.Fatalf("%+v", u)
	}
	// password sementara dari respons harus cocok dengan hash
	body := w.Body.String()
	i := strings.Index(body, `"temporary_password":"`)
	if i < 0 {
		t.Fatalf("tidak ada temporary_password: %s", body)
	}
	tp := body[i+len(`"temporary_password":"`):]
	tp = tp[:strings.Index(tp, `"`)]
	if okv, _ := sh.Hasher.Verify(u.PasswordHash, tp); !okv {
		t.Fatal("hash tidak cocok")
	}
}

func TestCreateUser_DuplicateEmailCaseInsensitive(t *testing.T) {
	h, _, _ := integRouter(t)
	if w := post(h, `{"email":"Sari@Tim.ID","name":"S","role":"developer"}`); w.Code != 201 {
		t.Fatalf("code=%d", w.Code)
	}
	if w := post(h, `{"email":"sari@tim.id","name":"S","role":"developer"}`); w.Code != 409 {
		t.Fatalf("code=%d body=%s", w.Code, w.Body)
	}
}

func TestCreateUser_ConcurrentSameEmail(t *testing.T) {
	h, _, _ := integRouter(t)
	var wg sync.WaitGroup
	codes := make(chan int, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes <- post(h, `{"email":"c@tim.id","name":"C","role":"developer"}`).Code
		}()
	}
	wg.Wait()
	close(codes)
	created := 0
	for c := range codes {
		switch c {
		case 201:
			created++
		case 409:
		default:
			t.Fatalf("code=%d", c)
		}
	}
	if created != 1 {
		t.Fatalf("created=%d", created)
	}
}

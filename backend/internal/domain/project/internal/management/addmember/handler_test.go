package addmember

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/project/internal/management/base"
)

type fakeExec struct {
	dto   base.MemberDTO
	err   error
	calls int
	pid   string
	in    AddMemberRequest
	meta  base.RequestMeta
}

func (f *fakeExec) Execute(_ context.Context, p string, in AddMemberRequest, m base.RequestMeta) (base.MemberDTO, error) {
	f.calls++
	f.pid, f.in, f.meta = p, in, m
	return f.dto, f.err
}

func serve(f *fakeExec, target, body, ua string) *httptest.ResponseRecorder {
	r := chi.NewRouter()
	r.Post("/projects/{projectID}/members", newHandler(f).ServeHTTP)
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	req.Header.Set("User-Agent", ua)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestHandler_201(t *testing.T) {
	name, email, by, label := "Citra", "c@tim.id", "a", "a@tim.id"
	f := &fakeExec{dto: base.MemberDTO{UserID: "u", Name: &name, Email: &email, Found: true, IsActive: true, Role: "member",
		AddedAt: base.SystemClock(), AddedBy: &by, AddedByLabel: &label}}
	w := serve(f, "/projects/p1/members", `{"user_id":"u"}`, strings.Repeat("ä", 600))
	if w.Code != 201 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	for _, k := range []string{`"user_id":"u"`, `"is_active":true`, `"found":true`, `"added_by_label":"a@tim.id"`, `"added_at":"`} {
		if !strings.Contains(w.Body.String(), k) {
			t.Fatalf("missing %s: %s", k, w.Body.String())
		}
	}
	if f.pid != "p1" || f.in.UserID != "u" || f.meta.IP == "" || len([]rune(f.meta.UserAgent)) != 512 {
		t.Fatalf("%+v %+v", f.in, f.meta)
	}
}

func TestHandler_BodyErrors(t *testing.T) {
	for _, body := range []string{"", "{rusak", `{"userId":"x"}`, `{"user_id":5}`, `{"user_id":""}`, `{"user_id":"  "}`} {
		f := &fakeExec{}
		w := serve(f, "/projects/p1/members", body, "ua")
		if w.Code != 400 || f.calls != 0 || !strings.Contains(w.Body.String(), "INVALID_ARGUMENT") {
			t.Fatalf("%q: %d %s", body, w.Code, w.Body.String())
		}
	}
	w := serve(&fakeExec{}, "/projects/p1/members", `{"user_id":""}`, "ua")
	if !strings.Contains(w.Body.String(), `"user_id":"wajib diisi"`) {
		t.Fatal(w.Body.String())
	}
}

func TestHandler_ErrorMapping(t *testing.T) {
	cases := []struct {
		err    error
		code   int
		substr string
	}{
		{base.ErrForbidden, 403, "FORBIDDEN"},
		{base.ErrProjectNotFound, 404, "NOT_FOUND"},
		{base.ErrAlreadyMember, 409, "ALREADY_MEMBER"},
		{base.UserNotEligible(base.ReasonUserInactive), 400, "USER_INACTIVE"},
		{errors.New("secret db detail"), 500, "INTERNAL"},
	}
	for _, c := range cases {
		w := serve(&fakeExec{err: c.err}, "/projects/p1/members", `{"user_id":"u"}`, "ua")
		b := w.Body.String()
		if w.Code != c.code || !strings.Contains(b, c.substr) || strings.Contains(b, "secret") || !strings.Contains(b, "trace_id") {
			t.Fatalf("%v: %d %s", c.err, w.Code, b)
		}
	}
}

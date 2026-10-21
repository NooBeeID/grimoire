package verifykey

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"grimoire/internal/domain/project/internal/apikey/base"
	"grimoire/lib/contracts/project/apikey/verify"
)

const (
	keyID     = "0192f5a0-2222-7aaa-8bbb-000000000001"
	projectID = "0192f5a0-1111-7aaa-8bbb-000000000001"
	keyName   = "claude-code-budi"
)

type fakeRepo struct {
	calls      int
	lastPrefix string
	key        base.Key
	found      bool
	err        error
}

func (f *fakeRepo) FindByPrefix(ctx context.Context, prefix string) (base.Key, bool, error) {
	f.calls++
	f.lastPrefix = prefix
	if f.err != nil {
		return base.Key{}, false, f.err
	}
	if err := ctx.Err(); err != nil {
		return base.Key{}, false, err
	}
	return f.key, f.found, nil
}

func rawKey(t testing.TB) base.RawKey {
	t.Helper()
	r, err := base.GenerateRawKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func keyFor(raw base.RawKey, active bool) base.Key {
	k := base.Key{ID: keyID, ProjectID: projectID, Name: keyName, Prefix: raw.Prefix, SecretHash: base.HashSecret(raw.Secret)}
	if !active {
		now := time.Now().UTC()
		k.RevokedAt = &now
	}
	return k
}

func exec(t testing.TB, f *fakeRepo, raw string) verify.Response {
	t.Helper()
	res, err := NewUsecase(f).Execute(context.Background(), verify.Request{RawKey: raw})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	return res
}

func TestVerify_ValidActive(t *testing.T) {
	raw := rawKey(t)
	f := &fakeRepo{key: keyFor(raw, true), found: true}
	res := exec(t, f, raw.Full)
	want := verify.Response{Found: true, KeyID: keyID, KeyName: keyName, Prefix: raw.Prefix, ProjectID: projectID}
	if res != want {
		t.Fatalf("got %+v want %+v", res, want)
	}
	if f.calls != 1 || f.lastPrefix != raw.Prefix {
		t.Fatalf("calls=%d prefix=%q", f.calls, f.lastPrefix)
	}
}

func TestT04_AC3_RevokedKeyNotFound(t *testing.T) {
	raw := rawKey(t)
	res := exec(t, &fakeRepo{key: keyFor(raw, false), found: true}, raw.Full)
	want := verify.Response{Found: false, Reason: verify.ReasonRevoked, Prefix: raw.Prefix}
	if res != want {
		t.Fatalf("got %+v want %+v", res, want)
	}
}

func TestT04_AC8_WrongSecretIsMismatch(t *testing.T) {
	raw, other := rawKey(t), rawKey(t)
	f := &fakeRepo{key: keyFor(raw, true), found: true}
	res := exec(t, f, "gr_"+raw.Prefix+"_"+other.Secret)
	want := verify.Response{Found: false, Reason: verify.ReasonMismatch, Prefix: raw.Prefix}
	if res != want {
		t.Fatalf("got %+v want %+v", res, want)
	}
}

func TestVerify_WrongSecretOnRevokedKeyIsMismatch(t *testing.T) {
	raw, other := rawKey(t), rawKey(t)
	res := exec(t, &fakeRepo{key: keyFor(raw, false), found: true}, "gr_"+raw.Prefix+"_"+other.Secret)
	if res.Reason != verify.ReasonMismatch || res.Found {
		t.Fatalf("got %+v", res)
	}
}

func TestVerify_UnknownPrefix(t *testing.T) {
	raw := rawKey(t)
	res := exec(t, &fakeRepo{}, raw.Full)
	want := verify.Response{Found: false, Reason: verify.ReasonUnknown, Prefix: raw.Prefix}
	if res != want {
		t.Fatalf("got %+v want %+v", res, want)
	}
}

func TestVerify_MalformedNeverHitsRepo(t *testing.T) {
	raw := rawKey(t)
	// Karakter terakhir secret 43 char hanya 4 bit berarti; ubah ke bentuk dengan bit sisa non-nol.
	last := raw.Secret[len(raw.Secret)-1]
	alt := byte('B')
	if last == 'B' {
		alt = 'C'
	}
	nonCanonical := raw.Full[:len(raw.Full)-1] + string(alt)
	if nonCanonical[len(nonCanonical)-1] == raw.Full[len(raw.Full)-1] {
		t.Fatal("setup")
	}
	cases := map[string]string{
		"empty":         "",
		"short":         "abc",
		"scheme-only":   "gr_",
		"truncated":     raw.Full[:len(raw.Full)-1],
		"extended":      raw.Full + "A",
		"scheme-upper":  "GR_" + raw.Full[3:],
		"scheme-other":  "gx_" + raw.Full[3:],
		"prefix-upper":  "gr_" + strings.ToUpper(raw.Prefix) + raw.Full[11:],
		"second-sep":    raw.Full[:11] + "-" + raw.Full[12:],
		"secret-eq":     raw.Full[:len(raw.Full)-1] + "=",
		"secret-plus":   raw.Full[:20] + "+" + raw.Full[21:],
		"secret-slash":  raw.Full[:20] + "/" + raw.Full[21:],
		"non-canonical": nonCanonical,
		"too-long":      strings.Repeat("a", 300),
		"bad-length":    "gr_" + strings.Repeat("a", 8) + "_" + strings.Repeat("A", 40),
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			// non-canonical hanya malformed bila bit sisa non-nol; pilih alt agar pasti demikian.
			f := &fakeRepo{}
			res, err := NewUsecase(f).Execute(context.Background(), verify.Request{RawKey: in})
			if err != nil || res.Found || res.Reason != verify.ReasonMalformed || res.Prefix != "" || f.calls != 0 {
				t.Fatalf("res=%+v err=%v calls=%d", res, err, f.calls)
			}
		})
	}
}

func TestVerify_SurroundingWhitespaceAccepted(t *testing.T) {
	raw := rawKey(t)
	res := exec(t, &fakeRepo{key: keyFor(raw, true), found: true}, "  "+raw.Full+"\n")
	if !res.Found || res.KeyID != keyID {
		t.Fatalf("got %+v", res)
	}
}

func TestT04_AC8_FailClosedOnRepoError(t *testing.T) {
	raw := rawKey(t)
	sentinel := errors.New("conn refused")
	res, err := NewUsecase(&fakeRepo{err: sentinel}).Execute(context.Background(), verify.Request{RawKey: raw.Full})
	if err == nil || res != (verify.Response{}) || !errors.Is(err, sentinel) {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if !strings.HasPrefix(err.Error(), "verifykey: ") {
		t.Fatalf("err=%q", err)
	}
	for _, s := range []string{raw.Full, raw.Secret, raw.Prefix} {
		if strings.Contains(err.Error(), s) {
			t.Fatalf("error leaks %q: %v", s, err)
		}
	}
}

func TestVerify_ContextCanceled(t *testing.T) {
	raw := rawKey(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f := &fakeRepo{key: keyFor(raw, true), found: true}
	res, err := NewUsecase(f).Execute(ctx, verify.Request{RawKey: raw.Full})
	if err == nil || res.Found {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestVerify_NoSecretInResponse(t *testing.T) {
	raw, other := rawKey(t), rawKey(t)
	paths := []struct {
		f   *fakeRepo
		in  string
		tag string
	}{
		{&fakeRepo{key: keyFor(raw, true), found: true}, raw.Full, "valid"},
		{&fakeRepo{key: keyFor(raw, false), found: true}, raw.Full, "revoked"},
		{&fakeRepo{key: keyFor(raw, true), found: true}, "gr_" + raw.Prefix + "_" + other.Secret, "mismatch"},
		{&fakeRepo{}, raw.Full, "unknown"},
		{&fakeRepo{}, "junk", "malformed"},
	}
	for _, p := range paths {
		s := fmt.Sprintf("%+v", exec(t, p.f, p.in))
		if strings.Contains(s, raw.Secret) || strings.Contains(s, raw.Full) {
			t.Fatalf("%s leaks: %s", p.tag, s)
		}
	}
}

func TestVerify_UsecaseHasNoStateOrCache(t *testing.T) {
	typ := reflect.TypeFor[Usecase]()
	if typ.NumField() != 1 || typ.Field(0).Name != "repo" {
		t.Fatalf("Usecase fields: %v", typ)
	}
	raw := rawKey(t)
	f := &fakeRepo{key: keyFor(raw, true), found: true}
	uc := NewUsecase(f)
	r1, _ := uc.Execute(context.Background(), verify.Request{RawKey: raw.Full})
	f.key = keyFor(raw, false)
	r2, _ := uc.Execute(context.Background(), verify.Request{RawKey: raw.Full})
	if !r1.Found || r2.Found || r2.Reason != verify.ReasonRevoked || f.calls != 2 {
		t.Fatalf("r1=%+v r2=%+v calls=%d", r1, r2, f.calls)
	}
}

func TestVerify_ReasonConstantsMatchBase(t *testing.T) {
	if verify.ReasonMalformed != base.ReasonMalformed || verify.ReasonUnknown != base.ReasonUnknown ||
		verify.ReasonRevoked != base.ReasonRevoked || verify.ReasonMismatch != base.ReasonMismatch {
		t.Fatal("reason constants differ between contract and base")
	}
}

// Tidak paralel: mengubah variabel paket outcome.
func TestT04_AC8_StructuralHashBeforeStatusCheck(t *testing.T) {
	raw, other := rawKey(t), rawKey(t)
	var n int
	orig := outcome
	outcome = func(k base.Key, found bool, secret string) (bool, string) {
		n++
		return orig(k, found, secret)
	}
	t.Cleanup(func() { outcome = orig })

	cases := []struct {
		tag   string
		f     *fakeRepo
		in    string
		calls int
	}{
		{"unknown", &fakeRepo{}, raw.Full, 1},
		{"mismatch", &fakeRepo{key: keyFor(raw, true), found: true}, "gr_" + raw.Prefix + "_" + other.Secret, 1},
		{"revoked", &fakeRepo{key: keyFor(raw, false), found: true}, raw.Full, 1},
		{"valid", &fakeRepo{key: keyFor(raw, true), found: true}, raw.Full, 1},
		{"malformed", &fakeRepo{}, "junk", 0},
	}
	for _, c := range cases {
		n = 0
		exec(t, c.f, c.in)
		if n != c.calls {
			t.Fatalf("%s: outcome calls=%d want %d", c.tag, n, c.calls)
		}
		if c.tag != "malformed" && c.f.calls != 1 {
			t.Fatalf("%s: repo calls=%d", c.tag, c.f.calls)
		}
	}
}

func median(d []time.Duration) time.Duration {
	s := append([]time.Duration(nil), d...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	return s[len(s)/2]
}

func TestT04_AC8_CPUPathUnknownVsMismatchComparable(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	raw, other := rawKey(t), rawKey(t)
	unknownIn := "gr_" + "zzzzzzzz" + "_" + other.Secret
	mismatchIn := "gr_" + raw.Prefix + "_" + other.Secret
	fu := &fakeRepo{}
	fm := &fakeRepo{key: keyFor(raw, true), found: true}
	uu, um := NewUsecase(fu), NewUsecase(fm)
	ctx := context.Background()

	const N = 2000
	var du, dm []time.Duration
	for i := 0; i < N; i++ {
		t0 := time.Now()
		_, _ = uu.Execute(ctx, verify.Request{RawKey: unknownIn})
		a := time.Since(t0)
		t0 = time.Now()
		_, _ = um.Execute(ctx, verify.Request{RawKey: mismatchIn})
		b := time.Since(t0)
		if i >= N/10 {
			du, dm = append(du, a), append(dm, b)
		}
	}
	mu, mm := median(du), median(dm)
	t.Logf("median unknown=%v mismatch=%v", mu, mm)
	if mu < 100*time.Nanosecond || mm < 100*time.Nanosecond {
		t.Skip("median < 100ns; pengukuran tidak bermakna (struktural tetap menjaga)")
	}
	ratio := float64(mu) / float64(mm)
	diff := mu - mm
	if diff < 0 {
		diff = -diff
	}
	if (ratio < 0.5 || ratio > 2.0) && diff >= 20*time.Microsecond {
		t.Fatalf("timing berbeda: unknown=%v mismatch=%v ratio=%.2f", mu, mm, ratio)
	}
}

func benchVerify(b *testing.B, f *fakeRepo, in string) {
	uc := NewUsecase(f)
	ctx := context.Background()
	req := verify.Request{RawKey: in}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = uc.Execute(ctx, req)
	}
}

func BenchmarkVerify_Unknown(b *testing.B) {
	raw := rawKey(b)
	benchVerify(b, &fakeRepo{}, raw.Full)
}

func BenchmarkVerify_Mismatch(b *testing.B) {
	raw, other := rawKey(b), rawKey(b)
	benchVerify(b, &fakeRepo{key: keyFor(raw, true), found: true}, "gr_"+raw.Prefix+"_"+other.Secret)
}

func BenchmarkVerify_Revoked(b *testing.B) {
	raw := rawKey(b)
	benchVerify(b, &fakeRepo{key: keyFor(raw, false), found: true}, raw.Full)
}

func BenchmarkVerify_Valid(b *testing.B) {
	raw := rawKey(b)
	benchVerify(b, &fakeRepo{key: keyFor(raw, true), found: true}, raw.Full)
}

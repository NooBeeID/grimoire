package base

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"regexp"
	"strings"
	"testing"
)

var policy = PasswordPolicy{MinLength: 10}

func testHasher() Argon2Hasher {
	return Argon2Hasher{Params: Argon2Params{MemoryKiB: 64, Iterations: 1, Parallelism: 1}}
}

func TestPasswordPolicy(t *testing.T) {
	cases := []struct{ name, pw, email, want string }{
		{"9 rune", "123456789", "a@b.id", "minimal 10 karakter"},
		{"10 rune", "1234567890", "a@b.id", ""},
		{"multibyte per rune", strings.Repeat("é", 10), "a@b.id", ""},
		{"multibyte 9 rune", strings.Repeat("é", 9), "a@b.id", "minimal 10 karakter"},
		{"257 byte", strings.Repeat("a", 257), "a@b.id", "maksimal 256 byte"},
		{"256 byte", strings.Repeat("a", 256), "a@b.id", ""},
		{"sama email", "Budi@Tim.ID", "budi@tim.id", "password tidak boleh sama dengan email"},
		{"kosong", "", "a@b.id", "wajib diisi"},
	}
	for _, c := range cases {
		fe := policy.ValidateField("password", c.pw, c.email)
		if fe["password"] != c.want {
			t.Errorf("%s: %v", c.name, fe)
		}
		if err := policy.Validate(c.pw, c.email); (err == nil) != (c.want == "") {
			t.Errorf("%s: Validate err=%v", c.name, err)
		}
	}
	if fe := policy.ValidateField("new_password", "x", ""); fe["new_password"] == "" {
		t.Fatal("nama field harus dipakai")
	}
}

func TestPasswordPolicy_ValidateChange(t *testing.T) {
	if fe := policy.ValidateChange("", "", "a@b.id"); len(fe) != 2 {
		t.Fatalf("keduanya kosong: %v", fe)
	}
	if fe := policy.ValidateChange("lama", "1234567890", "a@b.id"); fe != nil {
		t.Fatalf("lolos: %v", fe)
	}
	if fe := policy.ValidateChange("lama", "123456789", "a@b.id"); fe["new_password"] != "minimal 10 karakter" {
		t.Fatal(fe)
	}
	if fe := policy.ValidateChange(Secret(strings.Repeat("a", 257)), "1234567890", "a@b.id"); fe["current_password"] != "maksimal 256 byte" {
		t.Fatal(fe)
	}
	if fe := policy.ValidateChange("lama", "A@B.id", "a@b.id"); fe["new_password"] == "" {
		t.Fatal("sama dengan email")
	}
}

func TestDifferentPassword(t *testing.T) {
	if DifferentPassword("abc", "abc") == nil {
		t.Fatal("sama harus ditolak")
	}
	if DifferentPassword("abc", "abd") != nil || DifferentPassword("abc", "ABC") != nil {
		t.Fatal("beda (case-sensitive) harus lolos")
	}
	if fe := AsFieldErrors(DifferentPassword("x", "x")); fe["new_password"] != "password baru harus berbeda dari password lama" {
		t.Fatal(fe)
	}
}

func TestArgon2_HashVerifyRoundTrip(t *testing.T) {
	h := testHasher()
	enc, err := h.Hash("rahasia-panjang")
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := h.Verify(enc, "rahasia-panjang"); !ok || err != nil {
		t.Fatalf("%v %v", ok, err)
	}
	enc2, _ := h.Hash("rahasia-panjang")
	if enc == enc2 {
		t.Fatal("salt harus acak")
	}
}

func TestArgon2_FormatPHC(t *testing.T) {
	enc, _ := testHasher().Hash("x")
	re := regexp.MustCompile(`^\$argon2id\$v=19\$m=64,t=1,p=1\$[A-Za-z0-9+/]{22}\$[A-Za-z0-9+/]{43}$`)
	if !re.MatchString(enc) {
		t.Fatalf("format: %s", enc)
	}
}

func TestArgon2_VerifyWrongPassword(t *testing.T) {
	h := testHasher()
	enc, _ := h.Hash("benar")
	if ok, err := h.Verify(enc, "salah"); ok || err != nil {
		t.Fatalf("%v %v", ok, err)
	}
}

func TestArgon2_VerifyCorruptHash(t *testing.T) {
	h := testHasher()
	for _, enc := range []string{"", "bukan-hash", "$argon2id$v=19$m=64,t=1,p=1$!!$!!", "$argon2i$v=19$m=64,t=1,p=1$YWJj$YWJj",
		"$argon2id$v=18$m=64,t=1,p=1$YWJjZGVmZ2hpamtsbW5vcA$YWJjZGVmZ2hpamtsbW5vcA", "$argon2id$v=19$m=0,t=1,p=1$YWJj$YWJjZGVmZ2hpamtsbW5vcA",
		"$argon2id$v=19$m=999999999,t=1,p=1$YWJj$YWJjZGVmZ2hpamtsbW5vcA"} {
		if ok, err := h.Verify(enc, "x"); ok || err == nil {
			t.Errorf("%q harus error", enc)
		}
	}
}

func TestArgon2_VerifyUsesParamsFromHash(t *testing.T) {
	a := Argon2Hasher{Params: Argon2Params{MemoryKiB: 64, Iterations: 1, Parallelism: 1}}
	b := Argon2Hasher{Params: Argon2Params{MemoryKiB: 128, Iterations: 2, Parallelism: 2}}
	enc, _ := a.Hash("pw")
	if ok, err := b.Verify(enc, "pw"); !ok || err != nil {
		t.Fatalf("%v %v", ok, err)
	}
}

func TestGeneratePassword(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		pw, err := GeneratePassword(r)
		if err != nil || len(pw) != 16 {
			t.Fatalf("%q %v", pw, err)
		}
		if strings.ContainsAny(pw, "0O1lI") {
			t.Fatalf("karakter ambigu: %s", pw)
		}
		seen[pw] = true
	}
	if len(seen) != 1000 {
		t.Fatalf("unik: %d", len(seen))
	}
	a, _ := GeneratePassword(rand.New(rand.NewSource(7)))
	b, _ := GeneratePassword(rand.New(rand.NewSource(7)))
	if a != b {
		t.Fatal("deterministik dengan reader sama")
	}
	if _, err := GeneratePassword(bytes.NewReader(nil)); err == nil {
		t.Fatal("reader habis harus error")
	}
}

func TestSecret_NeverPrinted(t *testing.T) {
	s := Secret("super-rahasia-123")
	for _, out := range []string{
		fmt.Sprintf("%v", s), fmt.Sprintf("%s", s), fmt.Sprintf("%+v", s), fmt.Sprintf("%#v", s), fmt.Sprintf("%q", s),
		fmt.Sprintf("%v", struct{ P Secret }{s}),
	} {
		if strings.Contains(out, "super-rahasia") {
			t.Errorf("bocor: %s", out)
		}
	}
	b, _ := json.Marshal(struct{ P Secret }{s})
	if strings.Contains(string(b), "super") || !strings.Contains(string(b), "***") {
		t.Errorf("json: %s", b)
	}
	var buf bytes.Buffer
	slog.New(slog.NewJSONHandler(&buf, nil)).Info("x", "p", s)
	if strings.Contains(buf.String(), "super") {
		t.Errorf("slog: %s", buf.String())
	}
	var in struct{ P Secret }
	if err := json.Unmarshal([]byte(`{"P":"abc"}`), &in); err != nil || in.P.Reveal() != "abc" {
		t.Fatalf("unmarshal: %v %q", err, in.P)
	}
}

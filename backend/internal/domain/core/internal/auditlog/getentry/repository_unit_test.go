package getentry

import "testing"

func TestTrimHostMask(t *testing.T) {
	cases := map[string]string{
		"203.0.113.5": "203.0.113.5", "203.0.113.5/32": "203.0.113.5",
		"::1": "::1", "::1/128": "::1", "": "", "10.0.0.0/8": "10.0.0.0/8",
		"2001:db8::/32": "2001:db8::/32",
	}
	for in, want := range cases {
		if got := trimHostMask(in); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}

func TestDecodeJSON(t *testing.T) {
	m, err := decodeJSON([]byte(`null`))
	if err != nil || m == nil || len(m) != 0 {
		t.Fatalf("null: %v %v", m, err)
	}
	m, err = decodeJSON([]byte(`{"a":1,"b":[true,null],"c":{"d":"x"}}`))
	if err != nil || m["a"] != float64(1) {
		t.Fatalf("%v %v", m, err)
	}
	if _, err := decodeJSON([]byte(`[1]`)); err == nil {
		t.Fatal("array harus error")
	}
}

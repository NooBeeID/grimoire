package config

import (
	"os"
	"strings"
	"testing"
)

func TestEnvExample_Loads(t *testing.T) {
	b, err := os.ReadFile(repoFile(t, ".env.example"))
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]string{}
	for _, l := range strings.Split(string(b), "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		k, v, ok := strings.Cut(l, "=")
		if ok {
			m[k] = v
		}
	}
	if _, err := Load(env(m)); err != nil {
		t.Fatal(err)
	}
}

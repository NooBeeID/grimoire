package base

import "testing"

func ptr(s string) *string { return &s }

func TestActorOwnerScope(t *testing.T) {
	cases := []struct {
		name  string
		actor Actor
		scope OwnerScope
		admin bool
	}{
		{"developer", Actor{ID: "u-a", Role: "developer"}, OwnerScope{OwnerID: "u-a"}, false},
		{"super_admin", Actor{ID: "u-s", Role: "super_admin"}, OwnerScope{All: true}, true},
		{"role kosong", Actor{ID: "u-x", Role: ""}, OwnerScope{OwnerID: "u-x"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.actor.OwnerScope(); got != tc.scope {
				t.Fatalf("scope = %+v, want %+v", got, tc.scope)
			}
			if tc.actor.IsSuperAdmin() != tc.admin {
				t.Fatalf("IsSuperAdmin = %v", !tc.admin)
			}
		})
	}
}

func TestOwnerScopePermits(t *testing.T) {
	cases := []struct {
		name  string
		scope OwnerScope
		key   Key
		want  bool
	}{
		{"milik sendiri", OwnerScope{OwnerID: "u-a"}, Key{CreatedBy: ptr("u-a")}, true},
		{"milik orang lain", OwnerScope{OwnerID: "u-a"}, Key{CreatedBy: ptr("u-b")}, false},
		{"legacy developer", OwnerScope{OwnerID: "u-a"}, Key{}, false},
		{"legacy super_admin", OwnerScope{All: true}, Key{}, true},
		{"scope kosong", OwnerScope{}, Key{CreatedBy: ptr("")}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.scope.Permits(tc.key); got != tc.want {
				t.Fatalf("Permits = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestOwnerScopeEmpty(t *testing.T) {
	if !(OwnerScope{}).Empty() {
		t.Fatal("zero scope harus Empty")
	}
	if (OwnerScope{All: true}).Empty() || (OwnerScope{OwnerID: "u-a"}).Empty() {
		t.Fatal("scope valid tidak boleh Empty")
	}
}

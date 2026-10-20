package base

import (
	"errors"
	"testing"
)

func TestCheckRoleChange(t *testing.T) {
	adm := User{ID: "a", Role: RoleSuperAdmin, IsActive: true}
	dev := User{ID: "d", Role: RoleDeveloper, IsActive: true}
	cases := []struct {
		name   string
		actor  string
		target User
		role   string
		count  int
		want   error
	}{
		{"role sama", "a", adm, RoleSuperAdmin, 1, nil},
		{"self", "a", adm, RoleDeveloper, 5, ErrSelfAction},
		{"last admin", "x", adm, RoleDeveloper, 1, ErrLastSuperAdmin},
		{"admin kedua", "x", adm, RoleDeveloper, 2, nil},
		{"promote developer", "a", dev, RoleSuperAdmin, 1, nil},
		{"self lebih dulu dari last", "a", adm, RoleDeveloper, 1, ErrSelfAction},
		{"admin nonaktif", "x", User{ID: "n", Role: RoleSuperAdmin}, RoleDeveloper, 1, nil},
	}
	for _, c := range cases {
		if got := CheckRoleChange(c.actor, c.target, c.role, c.count); !errors.Is(got, c.want) && got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestCheckDeactivate(t *testing.T) {
	adm := User{ID: "a", Role: RoleSuperAdmin, IsActive: true}
	dev := User{ID: "d", Role: RoleDeveloper, IsActive: true}
	cases := []struct {
		name   string
		actor  string
		target User
		count  int
		want   error
	}{
		{"self", "a", adm, 3, ErrSelfAction},
		{"last admin", "x", adm, 1, ErrLastSuperAdmin},
		{"admin kedua", "x", adm, 2, nil},
		{"developer", "x", dev, 1, nil},
		{"sudah nonaktif self", "a", User{ID: "a", Role: RoleSuperAdmin}, 1, nil},
		{"self lebih dulu dari last", "a", adm, 1, ErrSelfAction},
	}
	for _, c := range cases {
		if got := CheckDeactivate(c.actor, c.target, c.count); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

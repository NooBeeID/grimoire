package base

import (
	"errors"
	"testing"
)

func TestCheckRoleChange(t *testing.T) {
	owner := Member{UserID: "a", Role: RoleOwner}
	member := Member{UserID: "b", Role: RoleMember}
	cases := []struct {
		name  string
		level Level
		tgt   Member
		role  string
		n     int
		want  error
	}{
		{"no-op", LevelMember, member, RoleMember, 1, nil},
		{"member tidak boleh", LevelMember, member, RoleOwner, 1, ErrForbidden},
		{"owner terakhir turun", LevelOwner, owner, RoleMember, 1, ErrLastOwner},
		{"owner turunkan owner lain", LevelOwner, owner, RoleMember, 2, nil},
		{"admin turunkan owner terakhir", LevelAdmin, owner, RoleMember, 1, nil},
		{"member jadi owner", LevelOwner, member, RoleOwner, 1, nil},
	}
	for _, c := range cases {
		if err := CheckRoleChange("a", c.level, c.tgt, c.role, c.n); !errors.Is(err, c.want) && err != c.want {
			t.Errorf("%s: %v", c.name, err)
		}
	}
	if err := CheckRoleChange("a", LevelOwner, member, "admin", 1); err == nil {
		t.Error("role invalid harus error")
	}
}

func TestCheckRemove(t *testing.T) {
	a := Member{UserID: "a", Role: RoleOwner}
	b := Member{UserID: "b", Role: RoleMember}
	cases := []struct {
		name  string
		actor string
		level Level
		tgt   Member
		n     int
		want  error
	}{
		{"member keluar sendiri", "b", LevelMember, b, 1, nil},
		{"member keluarkan orang", "b", LevelMember, a, 1, ErrForbidden},
		{"owner terakhir keluar", "a", LevelOwner, a, 1, ErrLastOwner},
		{"owner keluar ada owner lain", "a", LevelOwner, a, 2, nil},
		{"admin keluarkan owner terakhir", "s", LevelAdmin, a, 1, nil},
		{"owner keluarkan member", "a", LevelOwner, b, 1, nil},
	}
	for _, c := range cases {
		if err := CheckRemove(c.actor, c.level, c.tgt, c.n); !errors.Is(err, c.want) && err != c.want {
			t.Errorf("%s: %v", c.name, err)
		}
	}
}

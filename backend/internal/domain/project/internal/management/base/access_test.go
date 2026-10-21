package base

import (
	"errors"
	"testing"
)

func TestResolveLevel(t *testing.T) {
	cases := []struct {
		role, m string
		active  bool
		want    Level
	}{
		{"super_admin", "", true, LevelAdmin},
		{"super_admin", "owner", true, LevelAdmin},
		{"developer", "owner", true, LevelOwner},
		{"developer", "member", true, LevelMember},
		{"developer", "", true, LevelNone},
		{"developer", "owner", false, LevelNone},
		{"agent", "owner", true, LevelNone},
		{"developer", "aneh", true, LevelNone},
	}
	for _, c := range cases {
		if got := ResolveLevel(c.role, c.m, c.active); got != c.want {
			t.Errorf("%+v got %s", c, got)
		}
	}
}

func TestRequire(t *testing.T) {
	perms := []Permission{PermView, PermEdit, PermManageMembers, PermDelete}
	for _, p := range perms {
		for _, l := range []Level{LevelOwner, LevelAdmin} {
			if err := Require(l, p); err != nil {
				t.Errorf("%s/%d: %v", l, p, err)
			}
		}
		for _, l := range []Level{LevelNone, Level("x")} {
			if err := Require(l, p); !errors.Is(err, ErrProjectNotFound) {
				t.Errorf("%s/%d: %v", l, p, err)
			}
		}
		err := Require(LevelMember, p)
		if p == PermView || p == PermEdit {
			if err != nil {
				t.Errorf("member/%d: %v", p, err)
			}
		} else if !errors.Is(err, ErrForbidden) {
			t.Errorf("member/%d: %v", p, err)
		}
	}
	if !CanManageMembers(LevelOwner) || !CanManageMembers(LevelAdmin) || CanManageMembers(LevelMember) {
		t.Error("CanManageMembers")
	}
}

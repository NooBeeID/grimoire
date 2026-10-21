package changememberrole

import "testing"

func TestChangeRoleRequest_Validate(t *testing.T) {
	empty, owner := "", "owner"
	for name, r := range map[string]ChangeRoleRequest{"nil": {}, "empty": {Role: &empty}} {
		if err := r.Validate(); err == nil {
			t.Fatalf("%s: harus error", name)
		}
	}
	r := ChangeRoleRequest{Role: &owner}
	if r.Validate() != nil || r.Value() != "owner" {
		t.Fatal("owner valid")
	}
	if (ChangeRoleRequest{}).Value() != "" {
		t.Fatal("nil -> kosong")
	}
	bad := "admin"
	if (ChangeRoleRequest{Role: &bad}).Validate() != nil {
		t.Fatal("nilai tidak dicek di Validate")
	}
}

//go:build integration

package buslistgroups

import (
	"context"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"grimoire/internal/domain/project/internal/searchsynonym/base"
	"grimoire/internal/domain/project/internal/searchsynonym/base/store"
	"grimoire/internal/infra/db"
	"grimoire/lib/contracts/project/searchsynonym/listgroups"
)

func TestIntegration_OnlyRequestedProject(t *testing.T) {
	ctx := context.Background()
	d := db.TestDB(t)
	if _, err := db.Up(ctx, d, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	st := store.New(d)
	uc := New(NewRepo(st))
	actor := base.Actor{ID: uuid.NewString(), Label: "Budi", Role: "developer"}

	project := func(name string) string {
		var id string
		if err := d.GetContext(ctx, &id, `INSERT INTO projects (name) VALUES ($1) RETURNING id::text`, name); err != nil {
			t.Fatal(err)
		}
		return id
	}
	a, b, empty := project("Alpha"), project("Beta"), project("Kosong")
	for _, c := range []struct {
		p     string
		terms []string
	}{
		{a, []string{"login", "autentikasi"}},
		{b, []string{"refund", "pengembalian dana"}},
		{a, []string{"invoice", "tagihan", "faktur"}},
	} {
		if _, err := st.Insert(ctx, base.NewGroup(c.p, c.terms, actor)); err != nil {
			t.Fatal(err)
		}
	}

	res, err := uc.Execute(ctx, listgroups.Request{ProjectID: a})
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"login", "autentikasi"}, {"invoice", "tagihan", "faktur"}}
	if !reflect.DeepEqual(res.Groups, want) {
		t.Fatalf("A = %v", res.Groups)
	}
	res, err = uc.Execute(ctx, listgroups.Request{ProjectID: b})
	if err != nil || !reflect.DeepEqual(res.Groups, [][]string{{"refund", "pengembalian dana"}}) {
		t.Fatalf("B = %v %v", res.Groups, err)
	}
	res, err = uc.Execute(ctx, listgroups.Request{ProjectID: empty})
	if err != nil || res.Groups != nil {
		t.Fatalf("empty = %v %v", res.Groups, err)
	}
}

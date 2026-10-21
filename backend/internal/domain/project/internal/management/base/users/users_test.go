package users

import (
	"context"
	"errors"
	"testing"

	"grimoire/internal/infra/bus"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/auth/finddevelopers"
	"grimoire/lib/common/errs"
	"grimoire/lib/contracts/core/auth/getuser"
	"grimoire/lib/contracts/core/auth/inviteuser"
	"grimoire/lib/contracts/core/auth/resendinvitation"
)

func TestGet(t *testing.T) {
	b := bus.NewRegistry(nil)
	b.Register(constants.BUS_CORE_AUTH_GET_USER, func(_ context.Context, req any) (any, error) {
		return getuser.Response{Found: true, ID: req.(getuser.Request).UserID}, nil
	})
	r, err := New(b).Get(context.Background(), "u1")
	if err != nil || !r.Found || r.ID != "u1" {
		t.Fatalf("%+v %v", r, err)
	}
}

func TestGet_NotRegistered(t *testing.T) {
	_, err := New(bus.NewRegistry(nil)).Get(context.Background(), "u1")
	if !errors.Is(err, bus.ErrNotRegistered) {
		t.Fatal(err)
	}
	_, err = New(bus.NewRegistry(nil)).FindDevelopers(context.Background(), "a", 5)
	if !errors.Is(err, bus.ErrNotRegistered) {
		t.Fatal(err)
	}
}

func TestFindDevelopers(t *testing.T) {
	b := bus.NewRegistry(nil)
	b.Register(constants.BUS_CORE_AUTH_FIND_DEVELOPERS, func(_ context.Context, req any) (any, error) {
		q := req.(finddevelopers.Request)
		return finddevelopers.Response{Items: []finddevelopers.Developer{{ID: q.Query}}}, nil
	})
	items, err := New(b).FindDevelopers(context.Background(), "bud", 50)
	if err != nil || len(items) != 1 || items[0].ID != "bud" {
		t.Fatalf("%+v %v", items, err)
	}
}

func TestGetByEmail(t *testing.T) {
	b := bus.NewRegistry(nil)
	b.Register(constants.BUS_CORE_AUTH_GET_USER, func(_ context.Context, req any) (any, error) {
		r := req.(getuser.Request)
		if r.UserID != "" {
			t.Errorf("UserID harus kosong: %+v", r)
		}
		return getuser.Response{Found: true, Email: r.Email}, nil
	})
	r, err := New(b).GetByEmail(context.Background(), "a@x.id")
	if err != nil || !r.Found || r.Email != "a@x.id" {
		t.Fatalf("%+v %v", r, err)
	}
}

func TestInvite_ForwardsRequestAndKeepsErrsError(t *testing.T) {
	b := bus.NewRegistry(nil)
	var got inviteuser.Request
	b.Register(constants.BUS_CORE_AUTH_INVITE_USER, func(_ context.Context, req any) (any, error) {
		got = req.(inviteuser.Request)
		if got.Email == "dup@x.id" {
			return nil, errs.WithDetails(errs.CodeAlreadyRegistered, "email sudah terdaftar", map[string]any{"reason": "registered"})
		}
		return inviteuser.Response{UserID: "u1", Created: true}, nil
	})
	in := inviteuser.Request{Email: "a@x.id", InvitedBy: "adm", InvitedByLabel: "Adm", ProjectName: "P"}
	res, err := New(b).Invite(context.Background(), in)
	if err != nil || res.UserID != "u1" || !res.Created || got != in {
		t.Fatalf("%+v %v %+v", res, err, got)
	}
	_, err = New(b).Invite(context.Background(), inviteuser.Request{Email: "dup@x.id"})
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodeAlreadyRegistered {
		t.Fatalf("errs.Error harus terbaca: %v", err)
	}
	_, err = New(bus.NewRegistry(nil)).Invite(context.Background(), in)
	if !errors.Is(err, bus.ErrNotRegistered) {
		t.Fatal(err)
	}
}

func TestResendInvitation_ForwardsRequestAndKeepsErrsError(t *testing.T) {
	b := bus.NewRegistry(nil)
	var got resendinvitation.Request
	b.Register(constants.BUS_CORE_AUTH_RESEND_INVITATION, func(_ context.Context, req any) (any, error) {
		got = req.(resendinvitation.Request)
		if got.UserID == "limited" {
			return nil, errs.New(errs.CodeRateLimited, "terlalu banyak")
		}
		return resendinvitation.Response{EmailSent: true, EmailStatus: "sent"}, nil
	})
	in := resendinvitation.Request{UserID: "u1", InvitedBy: "adm", ProjectName: "P", Initial: true}
	res, err := New(b).ResendInvitation(context.Background(), in)
	if err != nil || !res.EmailSent || got != in {
		t.Fatalf("%+v %v %+v", res, err, got)
	}
	_, err = New(b).ResendInvitation(context.Background(), resendinvitation.Request{UserID: "limited"})
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodeRateLimited {
		t.Fatalf("errs.Error harus terbaca: %v", err)
	}
	_, err = New(bus.NewRegistry(nil)).ResendInvitation(context.Background(), in)
	if !errors.Is(err, bus.ErrNotRegistered) {
		t.Fatal(err)
	}
}

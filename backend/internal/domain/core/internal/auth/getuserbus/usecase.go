// Package getuserbus menyediakan handler bus core.auth.get-user.
package getuserbus

import (
	"context"
	"fmt"
	"strings"
	"time"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/lib/contracts/core/auth/getuser"
)

type Usecase struct {
	repo  userFinder
	clock base.Clock
}

func NewUsecase(repo userFinder, clock base.Clock) *Usecase {
	if clock == nil {
		clock = base.SystemClock
	}
	return &Usecase{repo: repo, clock: clock}
}

// Execute mencari user. UserID diprioritaskan; Email dipakai hanya bila UserID kosong.
// Tidak ada / bukan uuid / email tidak valid -> Found=false tanpa error. User nonaktif tetap Found=true.
func (u *Usecase) Execute(ctx context.Context, in getuser.Request) (getuser.Response, error) {
	var (
		user  base.User
		found bool
		err   error
	)
	if id := strings.TrimSpace(in.UserID); id != "" {
		user, found, err = u.repo.FindUserByID(ctx, id)
		if err != nil {
			return getuser.Response{}, fmt.Errorf("getuserbus: find: %w", err)
		}
	} else {
		email, nerr := base.NormalizeEmail(in.Email)
		if nerr != nil {
			return getuser.Response{Found: false}, nil
		}
		user, found, err = u.repo.FindUserByEmail(ctx, email)
		if err != nil {
			return getuser.Response{}, fmt.Errorf("getuserbus: find by email: %w", err)
		}
	}
	if !found {
		return getuser.Response{Found: false}, nil
	}
	inv, hasInv, err := u.repo.FindInvitation(ctx, user.ID)
	if err != nil {
		return getuser.Response{}, fmt.Errorf("getuserbus: find invitation: %w", err)
	}
	res := getuser.Response{
		Found: true, ID: user.ID, Name: user.Name, Email: user.Email,
		Role: user.Role, IsActive: user.IsActive,
	}
	if hasInv {
		res.Invitation = toContract(inv, u.clock())
	}
	return res, nil
}

// toContract memetakan undangan ke kontrak bus; nil bila bukan undangan menunggu.
func toContract(i base.Invitation, now time.Time) *getuser.Invitation {
	if !i.IsPending() {
		return nil
	}
	return &getuser.Invitation{
		InvitedAt: i.CreatedAt, IssuedAt: i.IssuedAt, ExpiresAt: i.ExpiresAt, SentAt: i.SentAt,
		Expired: i.IsExpired(now), DeliveryStatus: i.DeliveryStatus, ResendCount: i.ResendCount,
	}
}

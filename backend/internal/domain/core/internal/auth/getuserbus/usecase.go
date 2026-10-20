// Package getuserbus menyediakan handler bus core.auth.get-user.
package getuserbus

import (
	"context"
	"fmt"
	"strings"

	"grimoire/lib/contracts/core/auth/getuser"
)

type Usecase struct{ repo userFinder }

func NewUsecase(repo userFinder) *Usecase { return &Usecase{repo: repo} }

// Execute mencari user; tidak ada / bukan uuid -> Found=false tanpa error.
// User nonaktif tetap Found=true.
func (u *Usecase) Execute(ctx context.Context, userID string) (getuser.Response, error) {
	user, found, err := u.repo.FindUserByID(ctx, strings.TrimSpace(userID))
	if err != nil {
		return getuser.Response{}, fmt.Errorf("getuserbus: find: %w", err)
	}
	if !found {
		return getuser.Response{Found: false}, nil
	}
	return getuser.Response{
		Found: true, ID: user.ID, Name: user.Name, Email: user.Email,
		Role: user.Role, IsActive: user.IsActive,
	}, nil
}

// Package checkaccess melayani bus project.management.check-access (keputusan akses project).
package checkaccess

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/internal/domain/project/internal/management/base/users"
	"grimoire/lib/common/errs"
	"grimoire/lib/contracts/core/auth/resolvesession"
	"grimoire/lib/contracts/project/management/checkaccess"
)

// AccessFinder dipenuhi base/store.Store.
type AccessFinder interface {
	FindAccess(ctx context.Context, projectID, userID string) (p base.Project, memberRole string, found bool, err error)
}

type Usecase struct {
	access AccessFinder
	users  users.Lookup
}

func NewUsecase(a AccessFinder, u users.Lookup) *Usecase { return &Usecase{access: a, users: u} }

func validate(req checkaccess.Request) error {
	if _, err := uuid.Parse(req.UserID); err != nil {
		return errs.Wrap(errs.CodeInvalid, "user_id tidak valid", errs.ErrInvalid)
	}
	if req.UserRole != resolvesession.RoleSuperAdmin && req.UserRole != resolvesession.RoleDeveloper {
		return errs.Wrap(errs.CodeInvalid, "user_role tidak valid", errs.ErrInvalid)
	}
	return nil
}

// Execute menentukan level akses; fail-closed bila ada error.
func (u *Usecase) Execute(ctx context.Context, req checkaccess.Request) (checkaccess.Response, error) {
	if err := validate(req); err != nil {
		return checkaccess.Response{}, err
	}
	_, memberRole, found, err := u.access.FindAccess(ctx, req.ProjectID, req.UserID)
	if err != nil {
		return checkaccess.Response{}, fmt.Errorf("checkaccess: find access: %w", err)
	}
	if !found {
		return checkaccess.Response{ProjectExists: false, Level: checkaccess.LevelNone}, nil
	}
	if req.UserRole == resolvesession.RoleSuperAdmin {
		return checkaccess.Response{ProjectExists: true, Level: base.ResolveLevel(req.UserRole, memberRole, true)}, nil
	}
	if memberRole == "" {
		return checkaccess.Response{ProjectExists: true, Level: checkaccess.LevelNone}, nil
	}
	usr, err := u.users.Get(ctx, req.UserID)
	if err != nil {
		return checkaccess.Response{}, fmt.Errorf("checkaccess: get-user: %w", err)
	}
	active := usr.Found && usr.IsActive
	return checkaccess.Response{ProjectExists: true, Level: base.ResolveLevel(req.UserRole, memberRole, active)}, nil
}

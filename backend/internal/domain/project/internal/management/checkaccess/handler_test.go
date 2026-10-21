package checkaccess

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"grimoire/internal/infra/bus"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/auth/getuser"
	"grimoire/lib/contracts/project/management/checkaccess"
)

func TestHandler_WrongTypes(t *testing.T) {
	h := newHandler(NewUsecase(&fakeAccess{}, &fakeUsers{}))
	for _, in := range []any{&checkaccess.Request{}, "x", nil} {
		if _, err := h.Handle(context.Background(), in); !errors.Is(err, bus.ErrBadRequestType) {
			t.Fatalf("%T: %v", in, err)
		}
	}
}

func TestHandler_ViaBusReturnsValue(t *testing.T) {
	b := bus.NewRegistry(nil)
	uc := NewUsecase(&fakeAccess{role: "member", found: true}, &fakeUsers{res: getuser.Response{Found: true, IsActive: true}})
	b.Register(constants.BUS_PROJECT_MANAGEMENT_CHECK_ACCESS, newHandler(uc).Handle)
	res, err := bus.CallTyped[checkaccess.Response](context.Background(), b, constants.BUS_PROJECT_MANAGEMENT_CHECK_ACCESS,
		checkaccess.Request{ProjectID: uuid.NewString(), UserID: uuid.NewString(), UserRole: "developer"})
	if err != nil || res != (checkaccess.Response{ProjectExists: true, Level: checkaccess.LevelMember}) {
		t.Fatalf("%+v %v", res, err)
	}
}

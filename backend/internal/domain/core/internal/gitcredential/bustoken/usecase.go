package bustoken

import (
	"context"
	"log/slog"
	"strings"

	"grimoire/internal/domain/core/internal/gitcredential/base"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/secret"
	"grimoire/lib/contracts/core/gitcredential/gettoken"
)

// opener dipenuhi *secretbox.Box (dan fake di tes).
type opener interface {
	Open(blob, aad []byte) ([]byte, error)
}

// tokenReader dipenuhi Repository (dan fake di tes).
type tokenReader interface {
	Get(ctx context.Context, userID string, p base.Provider, host string) (blob []byte, found bool, err error)
}

// Usecase membuka PAT milik satu user. Tanpa audit, tanpa Publish, tanpa log token.
type Usecase struct {
	repo tokenReader
	box  opener // nil interface = fitur nonaktif
	log  *slog.Logger
}

func NewUsecase(repo tokenReader, box opener, log *slog.Logger) *Usecase {
	if log == nil {
		log = slog.Default()
	}
	return &Usecase{repo: repo, box: box, log: log}
}

func (u *Usecase) Execute(ctx context.Context, in gettoken.Request) (gettoken.Response, error) {
	if !base.ValidUUID(in.UserID) {
		return gettoken.Response{}, errs.New(errs.CodeInvalid, "user_id tidak valid")
	}
	p, err := base.ParseProvider(in.Provider)
	if err != nil {
		return gettoken.Response{}, err
	}
	host := strings.ToLower(strings.TrimSpace(in.Host))
	if host == "" {
		return gettoken.Response{}, errs.New(errs.CodeInvalid, "host wajib diisi")
	}
	if u.box == nil {
		return gettoken.Response{Disabled: true}, nil
	}
	blob, found, err := u.repo.Get(ctx, in.UserID, p, host)
	if err != nil {
		return gettoken.Response{}, err
	}
	if !found {
		return gettoken.Response{}, nil
	}
	plain, err := u.box.Open(blob, base.AAD(in.UserID, p, host))
	if err != nil {
		u.log.WarnContext(ctx, "git credential unreadable",
			"user_id", in.UserID, "provider", string(p), "host", host)
		return gettoken.Response{Found: true, Unreadable: true}, nil
	}
	return gettoken.Response{Found: true, Token: secret.New(string(plain))}, nil
}

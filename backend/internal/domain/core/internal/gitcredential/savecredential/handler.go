package savecredential

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/core/internal/gitcredential/base"
	"grimoire/internal/infra/gitprovider"
	"grimoire/internal/infra/http/respond"
	"grimoire/internal/infra/middleware"
	"grimoire/lib/common/errs"
)

const maxRetryAfterSeconds = 3600

type Handler struct{ uc *Usecase }

func newHandler(uc *Usecase) *Handler { return &Handler{uc: uc} }

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")

	in, err := decodeBody(r)
	if err != nil {
		respond.WriteError(w, r, err)
		return
	}
	meta := base.NewRequestMeta(middleware.KeyByIP(r), r.UserAgent())
	res, err := h.uc.Execute(r.Context(), chi.URLParam(r, "provider"), in, meta)
	if err != nil {
		respond.WriteError(w, r, mapError(w, err))
		return
	}
	respond.WriteJSON(w, http.StatusOK, newResponse(res))
}

// decodeBody membaca body sendiri (bukan respond.DecodeJSON) agar *errs.Error dari
// SaveRequest.UnmarshalJSON (fields.token) tidak diratakan menjadi "JSON tidak valid".
func decodeBody(r *http.Request) (SaveRequest, error) {
	var in SaveRequest
	b, err := io.ReadAll(r.Body)
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return in, fmt.Errorf("body: %w", err)
		}
		return in, errs.New(errs.CodeInvalid, "JSON tidak valid")
	}
	if len(b) == 0 {
		return in, errs.New(errs.CodeInvalid, "body wajib diisi")
	}
	if err := json.Unmarshal(b, &in); err != nil {
		var e *errs.Error
		if errors.As(err, &e) {
			return in, e
		}
		return in, errs.New(errs.CodeInvalid, "JSON tidak valid")
	}
	return in, nil
}

// mapError memetakan VerifyFailure ke error HTTP; error lain diteruskan apa adanya.
func mapError(w http.ResponseWriter, err error) error {
	var vf *VerifyFailure
	if !errors.As(err, &vf) {
		return err
	}
	cause := vf.Cause
	switch {
	case errors.Is(cause, gitprovider.ErrUnauthorized), errors.Is(cause, gitprovider.ErrForbidden):
		return base.TokenInvalid(vf.Provider, vf.Host)
	case errors.Is(cause, gitprovider.ErrRateLimited):
		secs := 0
		var rl *gitprovider.RateLimitedError
		if errors.As(cause, &rl) && rl.RetryAfter > 0 {
			secs = min(int(math.Ceil(rl.RetryAfter.Seconds())), maxRetryAfterSeconds)
		}
		if secs > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(secs))
		}
		return base.RateLimited(secs)
	case errors.Is(cause, context.Canceled):
		return cause
	}
	return base.ErrUpstream
}

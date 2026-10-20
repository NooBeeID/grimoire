package respond

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
)

// TraceHeader adalah header trace id pada request/respons.
const TraceHeader = "X-Request-Id"

const genericInternal = "terjadi kesalahan internal"

type errBody struct {
	Code    errs.Code `json:"code"`
	Message string    `json:"message"`
	Details any       `json:"details"`
}

type errEnvelope struct {
	Error   errBody `json:"error"`
	TraceID string  `json:"trace_id"`
}

// WriteJSON menulis v sebagai JSON dengan status. Status 204/304 tanpa body.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	if status == http.StatusNoContent || status == http.StatusNotModified {
		w.WriteHeader(status)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("gagal menulis respons JSON", "error", err)
	}
}

func traceID(ctx context.Context, w http.ResponseWriter) string {
	if id := reqctx.TraceID(ctx); id != "" {
		return id
	}
	return w.Header().Get(TraceHeader)
}

// WriteError memetakan err ke respons error seragam. Detail error internal tidak
// pernah dikirim ke klien; hanya dicatat di log.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	status, body := mapError(err)
	if status >= 500 {
		lvl := slog.LevelError
		if errors.Is(err, context.Canceled) {
			lvl = slog.LevelWarn
		}
		slog.Default().Log(r.Context(), lvl, "request failed", "error", err, "status", status)
	}
	WriteJSON(w, status, errEnvelope{Error: body, TraceID: traceID(r.Context(), w)})
}

func mapError(err error) (int, errBody) {
	internal := func() (int, errBody) {
		return http.StatusInternalServerError, errBody{Code: errs.CodeInternal, Message: genericInternal}
	}
	var e *errs.Error
	if errors.As(err, &e) {
		if e.Code == errs.CodeInternal {
			return internal()
		}
		return StatusFor(e.Code), errBody{Code: e.Code, Message: e.Message, Details: e.Details}
	}
	sentinels := []struct {
		err  error
		code errs.Code
		msg  string
	}{
		{errs.ErrNotFound, errs.CodeNotFound, "resource tidak ditemukan"},
		{errs.ErrForbidden, errs.CodeForbidden, "akses ditolak"},
		{errs.ErrConflict, errs.CodeConflict, "konflik data"},
		{errs.ErrInvalid, errs.CodeInvalid, "permintaan tidak valid"},
		{errs.ErrUnauthorized, errs.CodeUnauthorized, "perlu login"},
	}
	for _, s := range sentinels {
		if errors.Is(err, s.err) {
			return StatusFor(s.code), errBody{Code: s.code, Message: s.msg}
		}
	}
	var mbe *http.MaxBytesError
	if errors.As(err, &mbe) {
		return http.StatusRequestEntityTooLarge, errBody{
			Code: errs.CodePayloadTooLarge, Message: "ukuran body melebihi batas",
			Details: map[string]int64{"limit_bytes": mbe.Limit},
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return http.StatusGatewayTimeout, errBody{Code: errs.CodeTimeout, Message: "permintaan melebihi batas waktu"}
	}
	return internal()
}

// DecodeJSON membaca satu objek JSON dari body ke dst; field tak dikenal ditolak.
func DecodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return mapDecodeErr(err)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return fmt.Errorf("body: %w", err)
		}
		return errs.New(errs.CodeInvalid, "JSON tidak valid")
	}
	return nil
}

func mapDecodeErr(err error) error {
	var (
		mbe *http.MaxBytesError
		syn *json.SyntaxError
		typ *json.UnmarshalTypeError
	)
	switch {
	case errors.As(err, &mbe):
		return fmt.Errorf("body: %w", err)
	case errors.Is(err, io.EOF):
		return errs.New(errs.CodeInvalid, "body wajib diisi")
	case errors.As(err, &syn), errors.Is(err, io.ErrUnexpectedEOF):
		return errs.New(errs.CodeInvalid, "JSON tidak valid")
	case errors.As(err, &typ):
		return errs.WithDetails(errs.CodeInvalid, "tipe field tidak sesuai", map[string]string{"field": typ.Field})
	}
	if f, ok := strings.CutPrefix(err.Error(), "json: unknown field "); ok {
		f = strings.Trim(f, `"`)
		return errs.WithDetails(errs.CodeInvalid, "field tidak dikenal", map[string]string{"field": f})
	}
	return errs.New(errs.CodeInvalid, "JSON tidak valid")
}

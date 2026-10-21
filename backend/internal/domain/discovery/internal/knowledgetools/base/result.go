package base

import (
	"errors"
	"fmt"

	"grimoire/lib/common/errs"
	"grimoire/lib/contracts/discovery/mcpgateway/toolkit"
)

// ArgResult membangun hasil isError untuk ArgError.
func ArgResult(e *ArgError, targetType, target string) toolkit.Result {
	return toolkit.Result{
		IsError:    true,
		Text:       e.Message(),
		TargetType: targetType,
		Target:     target,
		Summary:    e.Summary(),
	}
}

// BusResult memetakan error bus sesuai D13. ErrInvalid menjadi hasil isError (err nil); selain itu
// dikembalikan sebagai error Go (dibungkus %w) agar gateway memetakannya ke internal/timeout.
func BusResult(tool string, err error, hint, targetType, target string) (toolkit.Result, error) {
	if errors.Is(err, errs.ErrInvalid) {
		return toolkit.Result{
			IsError:    true,
			Text:       "Permintaan ditolak: " + err.Error() + ". " + hint,
			TargetType: targetType,
			Target:     target,
			Summary:    map[string]any{"error_code": "invalid_request"},
		}, nil
	}
	return toolkit.Result{}, fmt.Errorf("%s: %w", tool, err)
}

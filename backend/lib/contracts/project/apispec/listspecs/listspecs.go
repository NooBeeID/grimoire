// Package listspecs adalah kontrak bus project.apispec.list-specs (provider T06, consumer T09).
// ProjectID bukan uuid -> error membungkus errs.ErrInvalid. Project tanpa spec -> Items kosong (bukan nil).
// Tipe request salah -> bus.ErrBadRequestType. Request dikirim sebagai nilai.
//
// Provider memakai errs.Wrap(errs.CodeInvalid, "project_id tidak valid", errs.ErrInvalid);
// paket kontrak ini sengaja hanya stdlib (rule 4.5).
package listspecs

import "time"

type Request struct{ ProjectID string }

type Response struct{ Items []Spec } // urut Name

type Spec struct {
	Name, Title, ApiVersion, OpenapiVersion string
	EndpointCount                           int
	UpdatedAt                               time.Time
}

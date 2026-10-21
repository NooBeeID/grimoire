// Package getproject adalah kontrak bus project.management.get-project (provider T03, consumer T07).
// ProjectID bukan uuid atau tidak ada -> Found=false, err nil. Tipe request salah -> ErrBadRequestType.
package getproject

type Request struct{ ProjectID string }

type Response struct {
	Found bool
	ID    string
	Name  string
}

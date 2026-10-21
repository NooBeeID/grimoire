// Package upload membaca multipart upload spec secara streaming dengan batas ukuran per file dan jumlah file.
package upload

import (
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"path"
	"strings"

	"grimoire/internal/domain/project/internal/apispec/base"
	"grimoire/lib/common/errs"
)

const maxNameFieldBytes = 1024

type File struct {
	FileName string // basename untuk tampilan (setelah "\" -> "/")
	RawPath  string // filename mentah dari Content-Disposition (bisa berisi direktori)
	Content  []byte // nil bila TooLarge/Rejected
	TooLarge bool
	Rejected string // reason_code (TOO_MANY_FILES) bila part dibuang
}

type Form struct {
	Files []File
	Name  *string // field teks "name" (opsional)
}

func readErr(err error) error {
	var mbe *http.MaxBytesError
	if errors.As(err, &mbe) {
		return base.ErrPayloadTooLarge
	}
	return errs.Wrap(errs.CodeInvalid, "multipart tidak valid", err)
}

// Read membaca seluruh part sampai body habis. Field file bernama "files" (boleh berulang); field "name" teks.
// Body melebihi BodyLimit di tengah jalan -> ErrPayloadTooLarge untuk seluruh request.
func Read(r *http.Request, maxFileBytes int64, maxFiles int) (Form, error) {
	mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mt != "multipart/form-data" {
		return Form{}, base.ErrNotMultipart
	}
	mr, err := r.MultipartReader()
	if err != nil {
		return Form{}, base.ErrNotMultipart
	}
	var form Form
	fileParts := 0
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return Form{}, readErr(err)
		}
		switch part.FormName() {
		case "files":
			f, err := readFile(part, maxFileBytes, fileParts >= maxFiles)
			if err != nil {
				return Form{}, err
			}
			fileParts++
			form.Files = append(form.Files, f)
		case "name":
			b, err := io.ReadAll(io.LimitReader(part, maxNameFieldBytes+1))
			if err != nil {
				return Form{}, readErr(err)
			}
			if len(b) > maxNameFieldBytes {
				b = b[:maxNameFieldBytes]
				if _, err := io.Copy(io.Discard, part); err != nil {
					return Form{}, readErr(err)
				}
			}
			s := string(b)
			form.Name = &s
		default:
			if _, err := io.Copy(io.Discard, part); err != nil {
				return Form{}, readErr(err)
			}
		}
	}
	if len(form.Files) == 0 {
		return Form{}, base.ErrNoFiles
	}
	if form.Name != nil && len(form.Files) != 1 {
		return Form{}, base.ErrNameWithMultipleFiles
	}
	return form, nil
}

func readFile(part *multipart.Part, maxBytes int64, overCount bool) (File, error) {
	raw := part.FileName()
	if _, params, err := mime.ParseMediaType(part.Header.Get("Content-Disposition")); err == nil && params["filename"] != "" {
		raw = params["filename"] // BUKAN part.FileName(): itu membuang direktori
	}
	f := File{RawPath: raw, FileName: path.Base(strings.ReplaceAll(raw, "\\", "/"))}
	if overCount {
		f.Rejected = base.ReasonTooManyFiles
		if _, err := io.Copy(io.Discard, part); err != nil {
			return File{}, readErr(err)
		}
		return f, nil
	}
	b, err := io.ReadAll(io.LimitReader(part, maxBytes+1))
	if err != nil {
		return File{}, readErr(err)
	}
	if int64(len(b)) > maxBytes {
		f.TooLarge = true
		if _, err := io.Copy(io.Discard, part); err != nil {
			return File{}, readErr(err)
		}
		return f, nil
	}
	f.Content = b
	return f, nil
}

// Package uploadfiles menerima banyak file knowledge sekaligus (multipart streaming) untuk satu project.
package uploadfiles

import (
	"bytes"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"

	"grimoire/internal/domain/project/internal/knowledge/base"
)

const (
	formField = "files"
	bomBytes  = 3
)

// RawPart adalah hasil baca satu part file multipart (sebelum klasifikasi).
type RawPart struct {
	Index     int    // urutan part file di request, 0-based
	Name      string // filename MENTAH dari Content-Disposition (bisa memuat "/")
	Data      []byte // maks MaxFileBytes+3 byte; nil bila TooLarge atau OverCount
	TooLarge  bool   // part melebihi MaxFileBytes+3 byte
	OverCount bool   // part ke-(MaxUploadFiles+1) dst: tidak disimpan
}

// Parsed adalah hasil pembacaan seluruh body.
type Parsed struct {
	Parts      []RawPart
	TotalBytes int64 // byte mentah semua part file, termasuk yang dikuras
}

func tooLargeErr(lim base.Limits) error {
	return base.TooLarge(base.ReasonUploadTooLarge, lim.MaxUploadBytes)
}

// mapReadErr: MaxBytesError -> 413; error multipart lain -> 400 not_multipart.
func mapReadErr(err error, lim base.Limits) error {
	var mbe *http.MaxBytesError
	if errors.As(err, &mbe) {
		return tooLargeErr(lim)
	}
	return base.Invalid(base.ReasonNotMultipart)
}

// drain membuang sisa part sambil menghitung byte; berhenti bila melewati budget.
func drain(r io.Reader, budget int64) (n int64, exceeded bool, err error) {
	if budget < 0 {
		budget = 0
	}
	n, err = io.Copy(io.Discard, io.LimitReader(r, budget+1))
	if err != nil {
		return n, false, err
	}
	return n, n > budget, nil
}

// fileName mengambil filename mentah. isFile=false bila part bukan file field "files".
// DILARANG memakai Part.FileName(): ia memanggil filepath.Base dan membuang folder.
func fileName(h map[string][]string, get func(string) string) (name string, isFile bool) {
	cd := get("Content-Disposition")
	_, params, err := mime.ParseMediaType(cd)
	if err != nil {
		// header rusak: tetap dihitung sebagai part file bernama kosong bila memuat filename.
		if strings.Contains(strings.ToLower(cd), "filename") && strings.Contains(cd, formField) {
			return "", true
		}
		return "", false
	}
	fn, ok := params["filename"]
	if !ok || params["name"] != formField {
		return "", false
	}
	return fn, true
}

// readParts membaca body multipart sebagai stream.
func readParts(r *http.Request, lim base.Limits) (Parsed, error) {
	mr, err := r.MultipartReader()
	if err != nil {
		return Parsed{}, base.Invalid(base.ReasonNotMultipart)
	}
	var out Parsed
	out.Parts = []RawPart{}
	over := func() bool { return out.TotalBytes > lim.MaxUploadBytes }
	maxData := lim.MaxFileBytes + bomBytes

	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return Parsed{}, mapReadErr(err, lim)
		}
		name, isFile := fileName(part.Header, part.Header.Get)
		if !isFile {
			if _, _, err := drain(part, 1<<62); err != nil {
				return Parsed{}, mapReadErr(err, lim)
			}
			continue
		}
		rp := RawPart{Index: len(out.Parts), Name: name}
		if len(out.Parts) >= lim.MaxUploadFiles {
			rp.OverCount = true
			n, _, err := drain(part, lim.MaxUploadBytes-out.TotalBytes)
			if err != nil {
				return Parsed{}, mapReadErr(err, lim)
			}
			out.TotalBytes += n
		} else {
			var buf bytes.Buffer
			n, err := io.Copy(&buf, io.LimitReader(part, maxData+1))
			if err != nil {
				return Parsed{}, mapReadErr(err, lim)
			}
			out.TotalBytes += n
			if over() {
				return Parsed{}, tooLargeErr(lim)
			}
			if n > maxData {
				rp.TooLarge = true
				d, _, err := drain(part, lim.MaxUploadBytes-out.TotalBytes)
				if err != nil {
					return Parsed{}, mapReadErr(err, lim)
				}
				out.TotalBytes += d
			} else {
				rp.Data = append([]byte(nil), buf.Bytes()...)
			}
		}
		if over() {
			return Parsed{}, tooLargeErr(lim)
		}
		out.Parts = append(out.Parts, rp)
	}
}

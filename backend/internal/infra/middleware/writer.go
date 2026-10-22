package middleware

import (
	"bufio"
	"errors"
	"net"
	"net/http"
)

// statusWriter mencatat status dan jumlah byte respons.
type statusWriter struct {
	http.ResponseWriter
	status      int
	bytes       int64
	wroteHeader bool
}

func newStatusWriter(w http.ResponseWriter) *statusWriter {
	if sw, ok := w.(*statusWriter); ok {
		return sw
	}
	return &statusWriter{ResponseWriter: w, status: http.StatusOK}
}

func (w *statusWriter) WriteHeader(code int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += int64(n)
	return n, err
}

func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		if !w.wroteHeader {
			w.WriteHeader(http.StatusOK)
		}
		f.Flush()
	}
}

// FlushError mengembalikan http.ErrNotSupported (tanpa menulis header) bila writer bawah
// tidak mendukung flush, sehingga handler SSE bisa menolak sebelum header terkirim.
func (w *statusWriter) FlushError() error {
	if !canFlush(w.ResponseWriter) {
		return http.ErrNotSupported
	}
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return http.NewResponseController(w.ResponseWriter).Flush()
}

func canFlush(rw http.ResponseWriter) bool {
	for {
		switch v := rw.(type) {
		case http.Flusher:
			return true
		case interface{ FlushError() error }:
			return true
		case interface{ Unwrap() http.ResponseWriter }:
			rw = v.Unwrap()
		default:
			return false
		}
	}
}

func (w *statusWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := w.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, errors.New("httpx: hijack tidak didukung")
}

// Unwrap untuk http.ResponseController.
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

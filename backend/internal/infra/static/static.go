// Package static menyajikan SPA frontend yang ter-embed (dengan fallback ke
// index.html) atau mem-proxy Vite dev server pada mode development.
package static

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"strings"
	"time"

	"grimoire/internal/infra/config"
	httpx "grimoire/internal/infra/http"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
)

//go:embed all:dist
var distFS embed.FS

const (
	indexName      = "index.html"
	cacheImmutable = "public, max-age=31536000, immutable"
	cacheNoCache   = "no-cache"
	viteDownMsg    = "vite dev server tidak berjalan; jalankan npm run dev di frontend/"
)

// Embedded mengembalikan sub-FS berakar di dist.
func Embedded() fs.FS {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}

type handler struct {
	fsys  fs.FS
	index []byte
	proxy http.Handler
}

// New membuat handler. fsys boleh nil -> Embedded(). Handler TIDAK mendaftar route;
// pasang sebagai NotFound root router.
func New(cfg config.Config, fsys fs.FS, log *slog.Logger) (http.Handler, error) {
	if log == nil {
		log = slog.Default()
	}
	if fsys == nil {
		fsys = Embedded()
	}
	idx, err := fs.ReadFile(fsys, indexName)
	if err != nil {
		return nil, errors.New("static: index.html tidak ditemukan di FS")
	}
	h := &handler{fsys: fsys, index: idx}
	if cfg.AppEnv == "development" && cfg.DevViteURL != "" {
		target, err := url.Parse(cfg.DevViteURL)
		if err != nil || target.Scheme == "" || target.Host == "" {
			return nil, fmt.Errorf("static: DEV_VITE_URL tidak valid: %q", cfg.DevViteURL)
		}
		h.proxy = newProxy(target, log)
	}
	return h, nil
}

func newProxy(target *url.URL, log *slog.Logger) http.Handler {
	return &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.Out.Host = target.Host
			pr.SetXForwarded()
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			log.Warn("vite dev server tidak terjangkau",
				"trace_id", reqctx.TraceID(r.Context()), "error", err)
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(viteDownMsg))
		},
	}
}

func isReserved(p string) bool {
	for _, pre := range []string{"/api", "/mcp"} {
		if p == pre || strings.HasPrefix(p, pre+"/") {
			return true
		}
	}
	return false
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.ContainsRune(r.URL.Path, 0) {
		http.NotFound(w, r)
		return
	}
	p := path.Clean("/" + r.URL.Path)

	// 1. /api dan /mcp tidak pernah dilayani static.
	if isReserved(p) {
		httpx.WriteError(w, r, errs.New(errs.CodeNotFound, "resource tidak ditemukan"))
		return
	}
	// 2. Proxy Vite (dev).
	if h.proxy != nil {
		h.proxy.ServeHTTP(w, r)
		return
	}
	// 3a. Hanya GET/HEAD.
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		httpx.WriteError(w, r, errs.New(errs.CodeMethodNotAllowed, "method tidak diizinkan"))
		return
	}

	key := strings.TrimPrefix(p, "/")
	if key == "" {
		key = indexName
	}
	underAssets := strings.HasPrefix(key, "assets/") || key == "assets"

	// 3. File ada.
	if st, err := fs.Stat(h.fsys, key); err == nil && !st.IsDir() {
		data, err := fs.ReadFile(h.fsys, key)
		if err == nil {
			h.serve(w, r, key, data, underAssets)
			return
		}
	}
	// 4. Di bawah /assets/ tapi tak ada.
	if underAssets {
		http.NotFound(w, r)
		return
	}
	// 5. Berekstensi tapi tak ada.
	if path.Ext(p) != "" {
		http.NotFound(w, r)
		return
	}
	// 6. Fallback SPA.
	h.serve(w, r, indexName, h.index, false)
}

func (h *handler) serve(w http.ResponseWriter, r *http.Request, key string, data []byte, underAssets bool) {
	hd := w.Header()
	if key == indexName {
		hd.Set("Content-Type", "text/html; charset=utf-8")
		hd.Set("Cache-Control", cacheNoCache)
	} else {
		if ct := mime.TypeByExtension(path.Ext(key)); ct != "" {
			hd.Set("Content-Type", ct)
		}
		if underAssets {
			hd.Set("Cache-Control", cacheImmutable)
		} else {
			hd.Set("Cache-Control", cacheNoCache)
		}
	}
	http.ServeContent(w, r, path.Base(key), zeroTime, bytes.NewReader(data))
}

var zeroTime time.Time

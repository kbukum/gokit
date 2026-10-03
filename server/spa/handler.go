package spa

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/security"
	"github.com/kbukum/gokit/util"
)

const maxIndexBytes = 1024 * 1024

// Config identifies API namespaces and fingerprinted files from the build manifest. File paths are relative to the supplied filesystem.
type Config struct {
	ReservedPrefixes []string
	ImmutableAssets  []string
	CSP              string
}

type handler struct {
	files     fs.FS
	static    http.Handler
	index     []byte
	reserved  []string
	immutable map[string]bool
	policy    *security.NonceCSP
}

// New accepts embed.FS, an fs.Sub view, or an os.Root.FS for confined on-disk serving. The caller owns the filesystem lifetime. The index is bounded to 1 MiB and uses {nonce} in script/style nonce attributes.
func New(files fs.FS, cfg Config) (http.Handler, error) {
	if util.IsNil(files) {
		return nil, apperrors.InvalidInput("assets", "SPA filesystem is required")
	}
	policy, err := security.NewNonceCSP(cfg.CSP)
	if err != nil {
		return nil, err
	}
	indexFile, err := files.Open("index.html")
	if err != nil {
		return nil, apperrors.New(apperrors.ErrCodeInternal, "failed to open SPA index").WithCause(err)
	}
	index, readErr := io.ReadAll(io.LimitReader(indexFile, maxIndexBytes+1))
	if err := errors.Join(readErr, indexFile.Close()); err != nil {
		return nil, apperrors.New(apperrors.ErrCodeInternal, "failed to read SPA index").WithCause(err)
	}
	if len(index) > maxIndexBytes {
		return nil, apperrors.InvalidInput("index", "SPA index exceeds 1 MiB")
	}
	immutable := make(map[string]bool, len(cfg.ImmutableAssets))
	for _, name := range cfg.ImmutableAssets {
		if !fs.ValidPath(name) || name == "index.html" {
			return nil, apperrors.InvalidInput("immutableAssets", "immutable assets must be valid non-index build paths")
		}
		info, err := fs.Stat(files, name)
		if err != nil || info.IsDir() {
			return nil, apperrors.InvalidInput("immutableAssets", "immutable asset must exist and be a file").WithCause(err)
		}
		immutable[name] = true
	}
	reserved := []string{"/api", "/rpc", "/assets", "/debug", "/metrics", "/health", "/healthz", "/livez", "/readyz", "/info"}
	for _, prefix := range cfg.ReservedPrefixes {
		if !strings.HasPrefix(prefix, "/") || prefix == "/" || path.Clean(prefix) != prefix {
			return nil, apperrors.InvalidInput("reservedPrefixes", "prefixes must be canonical non-root absolute paths")
		}
		reserved = append(reserved, prefix)
	}
	return &handler{files: files, static: http.FileServerFS(files), index: index, reserved: reserved, immutable: immutable, policy: policy}, nil
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.NotFound(w, r)
		return
	}
	name := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if name != "" && !fs.ValidPath(name) {
		http.NotFound(w, r)
		return
	}
	if name == "" || name == "index.html" {
		h.serveIndex(w, r)
		return
	}
	if info, err := fs.Stat(h.files, name); err == nil && !info.IsDir() {
		if h.immutable[name] {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		h.static.ServeHTTP(w, r)
		return
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		http.Error(w, "asset unavailable", http.StatusInternalServerError)
		return
	}
	for _, prefix := range h.reserved {
		if r.URL.Path == prefix || strings.HasPrefix(r.URL.Path, prefix+"/") {
			http.NotFound(w, r)
			return
		}
	}
	if strings.Contains(name, ".") || !strings.Contains(r.Header.Get("Accept"), "text/html") {
		http.NotFound(w, r)
		return
	}
	h.serveIndex(w, r)
}

func (h *handler) serveIndex(w http.ResponseWriter, r *http.Request) {
	nonce, policy := h.policy.Issue()
	body := bytes.ReplaceAll(h.index, []byte("{nonce}"), []byte(nonce))
	w.Header().Set("Content-Security-Policy", policy)
	w.Header().Set("Cache-Control", "no-cache, no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(body))
}

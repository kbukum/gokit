package session

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/url"
	"time"

	"github.com/kbukum/gokit/auth"
	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/util"
)

// Login is the bounded password login input. Credentials must never be logged.
type Login struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginVerifier supplies application-owned verification without embedding membership in sessions.
type LoginVerifier interface {
	VerifyLogin(context.Context, Login) (auth.Principal, error)
}
type LoginVerifierFunc func(context.Context, Login) (auth.Principal, error)

func (f LoginVerifierFunc) VerifyLogin(ctx context.Context, in Login) (auth.Principal, error) {
	return f(ctx, in)
}

// ErrorWriter is the outer transport's normalizer/problem response boundary.
type ErrorWriter func(http.ResponseWriter, *http.Request, error)

// Response is the browser session contract. Identity omits credential references and tokens.
type Response struct {
	Status    string         `json:"status"`
	Identity  auth.Principal `json:"identity"`
	ExpiresAt time.Time      `json:"expiresAt"`
	CSRFToken string         `json:"csrfToken"`
}

// HandlerConfig configures the browser session endpoints.
type HandlerConfig struct {
	// Origin is the exact HTTPS origin accepted for login.
	Origin string
	// Errors writes every failure through the outer transport's problem boundary.
	Errors ErrorWriter
	// Clock derives cookie Max-Age from each grant's absolute expiry.
	Clock util.Clock
}

type handler struct {
	backend Backend
	origin  string
	errors  ErrorWriter
	clock   util.Clock
}

// NewHandler mounts POST /auth/login, GET /auth/session, and POST /auth/logout only.
// Login requires exact same-origin HTTPS and JSON; status never emits Set-Cookie. The handler validates every
// browser-facing input, including the cookie shape and logout CSRF header, before it calls the backend, so a backend may
// be the local Manager or a remote session authority.
func NewHandler(backend Backend, cfg HandlerConfig) (http.Handler, error) {
	u, err := url.Parse(cfg.Origin)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || util.IsNil(backend) || cfg.Errors == nil || util.IsNil(cfg.Clock) {
		return nil, apperrors.InvalidInput("auth", "Backend, HTTPS origin, error writer and clock are required")
	}
	h := &handler{backend: backend, origin: cfg.Origin, errors: cfg.Errors, clock: cfg.Clock}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /auth/login", h.login)
	mux.HandleFunc("GET /auth/session", h.status)
	mux.HandleFunc("POST /auth/logout", h.logout)
	return mux, nil
}

// RequestCSRF returns the synchronizer token an unsafe request presents in exactly one X-CSRF-Token header.
// Safe methods return an empty token without error.
func RequestCSRF(r *http.Request) (string, error) {
	if !unsafe(r.Method) {
		return "", nil
	}
	values := r.Header.Values("X-CSRF-Token")
	if len(values) != 1 || values[0] == "" {
		return "", csrfInvalid()
	}
	return values[0], nil
}

func csrfInvalid() error {
	return apperrors.New(apperrors.ErrCodeForbidden, "CSRF verification failed").WithReason("CSRF_INVALID")
}

func (h *handler) login(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	credential, err := auth.ParseCredentials(r)
	if err != nil {
		h.errors(w, r, err)
		return
	}
	if credential.Kind != "" && credential.Kind != auth.Session {
		h.errors(w, r, auth.Failure("UNSUPPORTED_CREDENTIAL"))
		return
	}
	if credential.Kind == auth.Session {
		if tokenErr := ValidateToken(credential.Value); tokenErr != nil {
			h.errors(w, r, tokenErr)
			return
		}
	}
	origins := r.Header.Values("Origin")
	types := r.Header.Values("Content-Type")
	if len(origins) != 1 || origins[0] != h.origin || len(types) != 1 {
		h.errors(w, r, auth.Failure("LOGIN_ORIGIN_INVALID"))
		return
	}
	contentType, _, err := mime.ParseMediaType(types[0])
	if err != nil || contentType != "application/json" {
		h.errors(w, r, apperrors.InvalidInput("login", "JSON is required"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	reader := http.MaxBytesReader(w, r.Body, 4096)
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	var input Login
	if decodeErr := decoder.Decode(&input); decodeErr != nil {
		h.errors(w, r, apperrors.InvalidInput("login", "Invalid login input"))
		return
	}
	var trailing json.RawMessage
	if decoder.Decode(&trailing) != io.EOF || input.Username == "" || input.Password == "" {
		h.errors(w, r, apperrors.InvalidInput("login", "Invalid login input"))
		return
	}
	grant, err := h.backend.SignIn(ctx, SignIn{Login: input, Presented: credential.Value})
	input.Password = ""
	if err != nil {
		h.errors(w, r, err)
		return
	}
	if tokenErr := ValidateToken(grant.Token); tokenErr != nil {
		h.errors(w, r, apperrors.Internal(tokenErr))
		return
	}
	seconds, err := h.remaining(grant)
	if err != nil {
		h.errors(w, r, err)
		return
	}
	h.respond(w, r, grant, &http.Cookie{Name: auth.SessionCookie, Value: grant.Token, Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode, Path: "/", Expires: grant.Principal.ExpiresAt, MaxAge: seconds})
}

// remaining rejects grants that are not a valid, unexpired session identity with a CSRF token, whatever the backend
// returned.
func (h *handler) remaining(grant Grant) (int, error) {
	p := grant.Principal
	if p.Credential != auth.Session || p.Validate() != nil {
		return 0, auth.Failure("SESSION_INVALID")
	}
	remaining := p.ExpiresAt.Sub(h.clock.Now())
	if remaining <= 0 || grant.CSRFToken == "" {
		return 0, auth.Failure("SESSION_INVALID")
	}
	return max(1, int(remaining/time.Second)), nil
}

func (h *handler) sessionToken(r *http.Request) (string, error) {
	credential, err := auth.ParseCredentials(r)
	if err != nil {
		return "", err
	}
	if credential.Kind != auth.Session {
		return "", auth.Failure("MISSING_CREDENTIAL")
	}
	if err := ValidateToken(credential.Value); err != nil {
		return "", err
	}
	return credential.Value, nil
}

func (h *handler) status(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	ctx, cancel := context.WithTimeout(r.Context(), LookupBudget)
	defer cancel()
	r = r.WithContext(ctx)
	token, err := h.sessionToken(r)
	if err != nil {
		h.errors(w, r, err)
		return
	}
	grant, err := h.backend.Status(ctx, token)
	if err != nil {
		h.errors(w, r, err)
		return
	}
	if _, err := h.remaining(grant); err != nil {
		h.errors(w, r, err)
		return
	}
	h.respond(w, r, grant, nil)
}

// respond encodes the session response before setting cookie (when non-nil), so a failed encoding never issues a
// session cookie on an error response.
func (h *handler) respond(w http.ResponseWriter, r *http.Request, grant Grant, cookie *http.Cookie) {
	p := grant.Principal
	data, err := json.Marshal(Response{Status: "authenticated", Identity: p.Clone(), ExpiresAt: p.ExpiresAt, CSRFToken: grant.CSRFToken})
	if err != nil {
		h.errors(w, r, apperrors.Internal(err))
		return
	}
	if cookie != nil {
		http.SetCookie(w, cookie)
	}
	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(data); err != nil {
		return
	}
}

func (h *handler) logout(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	ctx, cancel := context.WithTimeout(r.Context(), MutationBudget)
	defer cancel()
	r = r.WithContext(ctx)
	credential, err := auth.ParseCredentials(r)
	if err != nil || credential.Kind != auth.Session {
		h.errors(w, r, auth.Failure("SESSION_INVALID"))
		return
	}
	if tokenErr := ValidateToken(credential.Value); tokenErr != nil {
		h.errors(w, r, tokenErr)
		return
	}
	csrf, err := RequestCSRF(r)
	if err != nil {
		h.errors(w, r, err)
		return
	}
	if err := h.backend.SignOut(ctx, SignOut{Token: credential.Value, CSRFToken: csrf}); err != nil {
		h.errors(w, r, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: auth.SessionCookie, Value: "", Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode, Path: "/", Expires: time.Unix(1, 0), MaxAge: -1})
	w.WriteHeader(http.StatusNoContent)
}

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

type handler struct {
	manager  *Manager
	verifier LoginVerifier
	origin   string
	errors   ErrorWriter
}

// NewHandler mounts POST /auth/login, GET /auth/session, and POST /auth/logout only.
// Login requires exact same-origin HTTPS and JSON; status never emits Set-Cookie.
func NewHandler(manager *Manager, verifier LoginVerifier, origin string, errors ErrorWriter) (http.Handler, error) {
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || manager == nil || util.IsNil(verifier) || errors == nil {
		return nil, apperrors.InvalidInput("auth", "Manager, verifier, HTTPS origin and error writer are required")
	}
	h := &handler{manager: manager, verifier: verifier, origin: origin, errors: errors}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /auth/login", h.login)
	mux.HandleFunc("GET /auth/session", h.status)
	mux.HandleFunc("POST /auth/logout", h.logout)
	return mux, nil
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
	attempt, err := h.manager.BeginLogin(ctx, credential.Value)
	if err != nil {
		h.errors(w, r, err)
		return
	}
	p, err := h.verifier.VerifyLogin(ctx, input)
	input.Password = ""
	if err != nil {
		if app, ok := apperrors.AsAppError(err); ok && app != nil && app.Code == apperrors.ErrCodeUnauthorized {
			h.errors(w, r, auth.Failure("LOGIN_INVALID"))
			return
		}
		h.errors(w, r, storeFailure(err))
		return
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		h.errors(w, r, storeFailure(ctxErr))
		return
	}
	issued, err := h.manager.CompleteLogin(ctx, attempt, p)
	if err != nil {
		h.errors(w, r, err)
		return
	}
	token, err := h.manager.CSRFToken(ctx, issued.Principal.Reference)
	if err != nil {
		h.errors(w, r, err)
		return
	}
	remaining := issued.Principal.ExpiresAt.Sub(h.manager.clock.Now())
	if remaining <= 0 {
		h.errors(w, r, auth.Failure("SESSION_INVALID"))
		return
	}
	seconds := max(1, int(remaining/time.Second))
	http.SetCookie(w, &http.Cookie{Name: auth.SessionCookie, Value: issued.Token, Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode, Path: "/", Expires: issued.Principal.ExpiresAt, MaxAge: seconds})
	h.respond(w, r, issued.Principal, token)
}

func (h *handler) status(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	ctx, cancel := context.WithTimeout(r.Context(), LookupBudget)
	defer cancel()
	r = r.WithContext(ctx)
	p, err := h.manager.Authenticate(r)
	if err != nil {
		h.errors(w, r, err)
		return
	}
	token, err := h.manager.CSRFToken(ctx, p.Reference)
	if err != nil {
		h.errors(w, r, err)
		return
	}
	h.respond(w, r, p, token)
}

func (h *handler) respond(w http.ResponseWriter, r *http.Request, p auth.Principal, token string) {
	data, err := json.Marshal(Response{Status: "authenticated", Identity: p.Clone(), ExpiresAt: p.ExpiresAt, CSRFToken: token})
	if err != nil {
		h.errors(w, r, apperrors.Internal(err))
		return
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
	if err := ValidateToken(credential.Value); err != nil {
		h.errors(w, r, err)
		return
	}
	ref := h.manager.protection.Digest(credential.Value)
	values := r.Header.Values("X-CSRF-Token")
	if len(values) != 1 {
		h.errors(w, r, apperrors.New(apperrors.ErrCodeForbidden, "CSRF verification failed").WithReason("CSRF_INVALID"))
		return
	}
	if err := h.manager.csrf.Verify(ref, values[0]); err != nil {
		h.errors(w, r, err)
		return
	}
	if err := h.manager.Logout(ctx, ref); err != nil {
		h.errors(w, r, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: auth.SessionCookie, Value: "", Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode, Path: "/", Expires: time.Unix(1, 0), MaxAge: -1})
	w.WriteHeader(http.StatusNoContent)
}

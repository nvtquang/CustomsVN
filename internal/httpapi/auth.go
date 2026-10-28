package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/nvtquang/CustomsVN/internal/auth"
	"github.com/nvtquang/CustomsVN/internal/ratelimit"
)

// CookieName is the session cookie.
const CookieName = "customsvn_session"

// SecureCookieEnv forces the Secure flag on the session cookie, for a server
// behind a TLS proxy that does not set X-Forwarded-Proto.
const SecureCookieEnv = "CUSTOMSVN_SECURE_COOKIE"

type ctxKey struct{}

// userOf is the logged-in user of a request, nil for a caller with the API
// token.
func userOf(r *http.Request) *auth.User {
	u, _ := r.Context().Value(ctxKey{}).(*auth.User)
	return u
}

// ownerOf is the conversation owner a request acts as: its user, or 0 (any)
// for an admin.
func ownerOf(r *http.Request) int64 {
	if u := userOf(r); u != nil {
		return u.ID
	}
	return 0
}

// sessionUser reads the session cookie.
func (s *Server) sessionUser(r *http.Request) (*auth.User, error) {
	c, err := r.Cookie(CookieName)
	if err != nil {
		return nil, auth.ErrNoSession
	}
	return s.Auth.Session(r.Context(), c.Value)
}

// sameOrigin rejects a state-changing request that a browser sent from
// another site. SameSite=Lax already keeps the cookie off cross-site POSTs;
// this is the second lock, for browsers and proxies that get that wrong.
// Sec-Fetch-Site is set by the browser and survives reverse proxies that
// rewrite Host; without it, Origin is compared with the host the client
// asked for.
func sameOrigin(r *http.Request) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	switch r.Header.Get("Sec-Fetch-Site") {
	case "same-origin", "none":
		return true
	case "cross-site", "same-site":
		return false
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		// Not a browser: a script with the cookie is the cookie's owner.
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	for _, host := range []string{r.Host, r.Header.Get("X-Forwarded-Host")} {
		if host != "" && strings.EqualFold(u.Host, host) {
			return true
		}
	}
	return false
}

// authed lets through a logged-in user; admin only an admin.
func (s *Server) authed(h http.HandlerFunc) http.Handler { return s.guard(h, false) }
func (s *Server) admin(h http.HandlerFunc) http.Handler  { return s.guard(h, true) }

func (s *Server) guard(h http.HandlerFunc, admin bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, err := s.sessionUser(r)
		if err != nil {
			if !errors.Is(err, auth.ErrNoSession) {
				s.Log.Error("session", "err", err)
			}
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "login required", "code": "login"})
			return
		}
		if !sameOrigin(r) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "cross-site request refused"})
			return
		}
		if admin && !u.Admin() {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "admins only", "code": "admin"})
			return
		}
		h(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, u)))
	})
}

// apiAccess guards MCP and /v1: the bearer token when one is set, or a
// logged-in session. With neither, they are closed: an open endpoint would
// spend the owner's model budget for anyone who finds it.
func (s *Server) apiAccess(h http.Handler) http.Handler {
	token := os.Getenv(TokenEnv)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if bearer, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
			if token != "" && subtle.ConstantTimeCompare([]byte(bearer), []byte(token)) == 1 {
				h.ServeHTTP(w, r)
				return
			}
		} else if u, err := s.sessionUser(r); err == nil && sameOrigin(r) {
			h.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, u)))
			return
		}
		w.Header().Set("WWW-Authenticate", `Bearer realm="customsvn"`)
		msg := "missing or wrong bearer token"
		if token == "" {
			msg = "set " + TokenEnv + " on the server to use this endpoint with a bearer token"
		}
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": map[string]string{"message": msg, "type": "invalid_request_error"}})
	})
}

func clientIP(r *http.Request) string {
	// RemoteAddr only: X-Forwarded-For is whatever the client wants it to
	// be unless a trusted proxy set it, and CustomsVN cannot know that.
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func secureRequest(r *http.Request) bool {
	return r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" || os.Getenv(SecureCookieEnv) == "1"
}

func setSession(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{Name: CookieName, Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Secure: secureRequest(r), MaxAge: int(auth.SessionTTL / time.Second)})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if !sameOrigin(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "cross-site request refused"})
		return
	}
	// Two buckets: one per address against guessing many accounts, one per
	// account against guessing from many addresses.
	for _, key := range []string{"ip:" + clientIP(r), "email:" + auth.NormEmail(in.Email)} {
		if ok, wait := s.loginLimit.Allow(key); !ok {
			tooMany(w, wait)
			return
		}
	}
	token, u, err := s.Auth.Login(r.Context(), in.Email, in.Password, r.UserAgent())
	if errors.Is(err, auth.ErrBadCredentials) {
		s.Log.Warn("login failed", "email", auth.NormEmail(in.Email), "ip", clientIP(r))
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error(), "code": "credentials"})
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	setSession(w, r, token)
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(CookieName); err == nil {
		_ = s.Auth.Logout(r.Context(), c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: CookieName, Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	u, err := s.sessionUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "login required", "code": "login"})
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	u := userOf(r)
	if ok, wait := s.loginLimit.Allow("email:" + u.Email); !ok {
		tooMany(w, wait)
		return
	}
	token, err := s.Auth.ChangePassword(r.Context(), u.ID, in.Current, in.New, r.UserAgent())
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, auth.ErrBadCredentials) {
			status = http.StatusForbidden
		}
		writeError(w, status, err)
		return
	}
	setSession(w, r, token)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	list, err := s.Auth.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email    string `json:"email"`
		Name     string `json:"name"`
		Role     string `json:"role"`
		Password string `json:"password"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	generated := in.Password == ""
	if generated {
		in.Password = auth.RandomPassword()
	}
	u, err := s.Auth.Create(r.Context(), in.Email, in.Name, in.Role, in.Password)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	out := map[string]any{"user": u}
	if generated {
		// Shown once to the admin, who hands it over; never stored in clear.
		out["password"] = in.Password
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) updateUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in auth.Update
	if !readJSON(w, r, &in) {
		return
	}
	u, err := s.Auth.Update(r.Context(), id, in)
	if err != nil {
		writeError(w, userStatus(err), err)
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) deleteUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if id == userOf(r).ID {
		writeError(w, http.StatusBadRequest, errors.New("you cannot delete your own account"))
		return
	}
	if err := s.Auth.Delete(r.Context(), id); err != nil {
		writeError(w, userStatus(err), err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func userStatus(err error) int {
	switch {
	case errors.Is(err, auth.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, auth.ErrLastAdmin):
		return http.StatusConflict
	}
	return http.StatusBadRequest
}

func tooMany(w http.ResponseWriter, wait time.Duration) {
	secs := int(wait.Seconds())
	w.Header().Set("Retry-After", strconv.Itoa(secs))
	writeJSON(w, http.StatusTooManyRequests, map[string]any{"error": fmt.Sprintf("too many requests; try again in %d s", secs),
		"code": "rate_limit", "retry_after": secs})
}

// Limits bound how many questions one user asks. They protect the model
// budget, not the server, which answers far more than a person asks.
type Limits struct {
	PerMinute int `json:"questions_per_minute"`
	PerDay    int `json:"questions_per_day"`
}

// DefaultLimits are generous for a person and stop a runaway script.
var DefaultLimits = Limits{PerMinute: 10, PerDay: 500}

func (s *Server) limits(ctx context.Context) Limits {
	l := DefaultLimits
	var raw []byte
	if err := s.Pool.QueryRow(ctx, `SELECT value FROM settings WHERE key = 'limits'`).Scan(&raw); err == nil {
		_ = json.Unmarshal(raw, &l)
	}
	return l
}

func (s *Server) getLimits(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.limits(r.Context()))
}

func (s *Server) putLimits(w http.ResponseWriter, r *http.Request) {
	var l Limits
	if !readJSON(w, r, &l) {
		return
	}
	if err := s.saveLimits(r.Context(), l); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, l)
}

func (s *Server) saveLimits(ctx context.Context, l Limits) error {
	if l.PerMinute < 0 || l.PerDay < 0 || l.PerMinute > 1000 || l.PerDay > 100000 {
		return errors.New("limits must be 0 (no limit) to 1000 a minute and 100000 a day")
	}
	raw, _ := json.Marshal(l)
	if _, err := s.Pool.Exec(ctx, `INSERT INTO settings (key, value) VALUES ('limits', $1)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`, raw); err != nil {
		return err
	}
	s.questionLimit.Set(l.PerMinute, l.PerMinute)
	return nil
}

// allowQuestion applies the limits to the caller of r, writing the refusal
// when the caller is over one.
func (s *Server) allowQuestion(w http.ResponseWriter, r *http.Request) bool {
	key, user := "token", int64(0)
	if u := userOf(r); u != nil {
		key, user = "user:"+strconv.FormatInt(u.ID, 10), u.ID
	}
	if ok, wait := s.questionLimit.Allow(key); !ok {
		tooMany(w, wait)
		return false
	}
	if user == 0 {
		return true
	}
	if l := s.limits(r.Context()); l.PerDay > 0 {
		var n int
		if err := s.Pool.QueryRow(r.Context(), `SELECT count(*) FROM messages m JOIN conversations c ON c.id = m.conversation_id
			WHERE c.user_id = $1 AND m.role = 'user' AND m.created_at > now() - interval '1 day'`, user).Scan(&n); err == nil && n >= l.PerDay {
			writeJSON(w, http.StatusTooManyRequests, map[string]any{"error": fmt.Sprintf("the limit of %d questions a day is reached", l.PerDay),
				"code": "daily_limit"})
			return false
		}
	}
	return true
}

func newLimiters() (login, questions *ratelimit.Limiter) {
	return ratelimit.New(10, 10), ratelimit.New(DefaultLimits.PerMinute, DefaultLimits.PerMinute)
}

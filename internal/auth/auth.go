// Package auth holds the accounts of the Web UI: users with a role, password
// hashes and login sessions.
package auth

import (
	"context"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Roles. An admin manages data, settings, users and reviews; a user asks.
const (
	RoleAdmin = "admin"
	RoleUser  = "user"
)

// SessionTTL is how long a session lasts without use. Each use extends it.
const SessionTTL = 30 * 24 * time.Hour

// MinPassword is the shortest password accepted.
const MinPassword = 10

// Iterations of PBKDF2-SHA256, the OWASP figure for 2023 onwards. Tests
// lower it.
var Iterations = 600_000

var (
	ErrBadCredentials = errors.New("wrong email or password")
	ErrNoSession      = errors.New("not logged in")
	ErrLastAdmin      = errors.New("the last active admin cannot be removed, disabled or demoted")
	ErrNotFound       = errors.New("no such user")
)

// User is an account as the UI sees it; the password hash never leaves the
// package.
type User struct {
	ID        int64      `json:"id"`
	Email     string     `json:"email"`
	Name      string     `json:"name"`
	Role      string     `json:"role"`
	Disabled  bool       `json:"disabled"`
	CreatedAt time.Time  `json:"created_at"`
	LastLogin *time.Time `json:"last_login,omitempty"`
}

// Admin reports whether the user may administer.
func (u *User) Admin() bool { return u != nil && u.Role == RoleAdmin }

// Store reads and writes users and sessions.
type Store struct {
	Pool *pgxpool.Pool
	Now  func() time.Time
}

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// HashPassword returns a self-describing PBKDF2-SHA256 hash:
// pbkdf2-sha256$iterations$salt$key, salt and key in base64.
func HashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, Iterations, 32)
	if err != nil {
		return "", err
	}
	enc := base64.RawStdEncoding
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s", Iterations, enc.EncodeToString(salt), enc.EncodeToString(key)), nil
}

// CheckPassword reports whether password matches hash.
func CheckPassword(hash, password string) bool {
	parts := strings.Split(hash, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return false
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter < 1 {
		return false
	}
	enc := base64.RawStdEncoding
	salt, err1 := enc.DecodeString(parts[2])
	want, err2 := enc.DecodeString(parts[3])
	if err1 != nil || err2 != nil {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, password, salt, iter, len(want))
	return err == nil && subtle.ConstantTimeCompare(got, want) == 1
}

// dummyHash is checked against when the email is unknown, so a login takes
// as long whether or not the account exists.
var dummyHash = sync.OnceValue(func() string {
	h, _ := HashPassword("not a password anyone has")
	return h
})

// RandomPassword returns a password for a new admin or a reset.
func RandomPassword() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func tokenHash(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// NormEmail lower-cases and trims an email. "admin" alone is accepted as a
// login name for the default account.
func NormEmail(email string) string { return strings.ToLower(strings.TrimSpace(email)) }

func validEmail(email string) error {
	if email == "admin" {
		return nil
	}
	if _, err := mail.ParseAddress(email); err != nil || strings.ContainsAny(email, "<> ") {
		return fmt.Errorf("invalid email %q", email)
	}
	return nil
}

func validPassword(p string) error {
	if len([]rune(p)) < MinPassword {
		return fmt.Errorf("a password needs at least %d characters", MinPassword)
	}
	return nil
}

// EnsureAdmin creates the first admin when there is no user at all, and hands
// it the conversations made before accounts existed. password may be empty:
// one is generated and returned, to be shown once.
func (s *Store) EnsureAdmin(ctx context.Context, email, password string) (created bool, pw string, err error) {
	var n int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&n); err != nil {
		return false, "", err
	}
	if n > 0 {
		return false, "", nil
	}
	if email = NormEmail(email); email == "" {
		email = "admin"
	}
	if password == "" {
		password = RandomPassword()
	}
	u, err := s.Create(ctx, email, "Admin", RoleAdmin, password)
	if err != nil {
		return false, "", err
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE conversations SET user_id = $1 WHERE user_id IS NULL`, u.ID); err != nil {
		return false, "", err
	}
	return true, password, nil
}

const userCols = `id, email, name, role, disabled, created_at, last_login`

func scanUser(row pgx.Row) (*User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.Disabled, &u.CreatedAt, &u.LastLogin)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &u, err
}

// Create adds a user.
func (s *Store) Create(ctx context.Context, email, name, role, password string) (*User, error) {
	email = NormEmail(email)
	if err := validEmail(email); err != nil {
		return nil, err
	}
	if role != RoleAdmin && role != RoleUser {
		return nil, fmt.Errorf("role must be %s or %s", RoleAdmin, RoleUser)
	}
	if err := validPassword(password); err != nil {
		return nil, err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return nil, err
	}
	u, err := scanUser(s.Pool.QueryRow(ctx, `INSERT INTO users (email, name, role, password_hash) VALUES ($1, $2, $3, $4)
		ON CONFLICT (email) DO NOTHING RETURNING `+userCols, email, strings.TrimSpace(name), role, hash))
	if errors.Is(err, ErrNotFound) {
		return nil, fmt.Errorf("a user with email %s exists", email)
	}
	return u, err
}

// List returns every user, admins first.
func (s *Store) List(ctx context.Context) ([]User, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+userCols+` FROM users ORDER BY role = 'admin' DESC, email`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *u)
	}
	return out, rows.Err()
}

// Get returns one user.
func (s *Store) Get(ctx context.Context, id int64) (*User, error) {
	return scanUser(s.Pool.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE id = $1`, id))
}

// Update is a change to a user; nil fields stay as they are.
type Update struct {
	Name     *string `json:"name,omitempty"`
	Role     *string `json:"role,omitempty"`
	Disabled *bool   `json:"disabled,omitempty"`
	Password *string `json:"password,omitempty"`
}

// Update changes a user. Disabling a user or setting a password ends the
// user's sessions. The last active admin cannot be disabled or demoted.
func (s *Store) Update(ctx context.Context, id int64, up Update) (*User, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	u, err := scanUser(tx.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE id = $1 FOR UPDATE`, id))
	if err != nil {
		return nil, err
	}
	wasAdmin := u.Role == RoleAdmin && !u.Disabled
	if up.Name != nil {
		u.Name = strings.TrimSpace(*up.Name)
	}
	if up.Role != nil {
		if *up.Role != RoleAdmin && *up.Role != RoleUser {
			return nil, fmt.Errorf("role must be %s or %s", RoleAdmin, RoleUser)
		}
		u.Role = *up.Role
	}
	if up.Disabled != nil {
		u.Disabled = *up.Disabled
	}
	if wasAdmin && (u.Role != RoleAdmin || u.Disabled) {
		if err := lastAdmin(ctx, tx, id); err != nil {
			return nil, err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE users SET name = $2, role = $3, disabled = $4 WHERE id = $1`, id, u.Name, u.Role, u.Disabled); err != nil {
		return nil, err
	}
	endSessions := u.Disabled
	if up.Password != nil {
		if err := validPassword(*up.Password); err != nil {
			return nil, err
		}
		hash, err := HashPassword(*up.Password)
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `UPDATE users SET password_hash = $2 WHERE id = $1`, id, hash); err != nil {
			return nil, err
		}
		endSessions = true
	}
	if endSessions {
		if _, err := tx.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1`, id); err != nil {
			return nil, err
		}
	}
	return u, tx.Commit(ctx)
}

// lastAdmin fails when no active admin other than id would remain.
func lastAdmin(ctx context.Context, tx pgx.Tx, id int64) error {
	var others int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM users WHERE role = 'admin' AND NOT disabled AND id <> $1`, id).Scan(&others); err != nil {
		return err
	}
	if others == 0 {
		return ErrLastAdmin
	}
	return nil
}

// Delete removes a user. Their conversations stay, without an owner.
func (s *Store) Delete(ctx context.Context, id int64) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lastAdmin(ctx, tx, id); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return tx.Commit(ctx)
}

// ChangePassword sets a user's own password after checking the current one.
// Other sessions of the user end; the one returned replaces the caller's.
func (s *Store) ChangePassword(ctx context.Context, id int64, current, next, userAgent string) (string, error) {
	var hash *string
	if err := s.Pool.QueryRow(ctx, `SELECT password_hash FROM users WHERE id = $1`, id).Scan(&hash); err != nil {
		return "", err
	}
	if hash == nil || !CheckPassword(*hash, current) {
		return "", ErrBadCredentials
	}
	if _, err := s.Update(ctx, id, Update{Password: &next}); err != nil {
		return "", err
	}
	return s.newSession(ctx, id, userAgent)
}

// Login checks a password and opens a session. It returns the token for the
// cookie; only its hash is stored.
func (s *Store) Login(ctx context.Context, email, password, userAgent string) (string, *User, error) {
	var hash *string
	u, err := scanUserHash(s.Pool.QueryRow(ctx, `SELECT `+userCols+`, password_hash FROM users WHERE email = $1`, NormEmail(email)), &hash)
	if errors.Is(err, ErrNotFound) {
		CheckPassword(dummyHash(), password)
		return "", nil, ErrBadCredentials
	}
	if err != nil {
		return "", nil, err
	}
	if hash == nil || !CheckPassword(*hash, password) || u.Disabled {
		return "", nil, ErrBadCredentials
	}
	token, err := s.newSession(ctx, u.ID, userAgent)
	if err != nil {
		return "", nil, err
	}
	now := s.now()
	u.LastLogin = &now
	_, err = s.Pool.Exec(ctx, `UPDATE users SET last_login = $2 WHERE id = $1`, u.ID, now)
	return token, u, err
}

func scanUserHash(row pgx.Row, hash **string) (*User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.Disabled, &u.CreatedAt, &u.LastLogin, hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &u, err
}

func (s *Store) newSession(ctx context.Context, user int64, userAgent string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(b)
	if r := []rune(userAgent); len(r) > 200 {
		userAgent = string(r[:200])
	}
	now := s.now()
	_, err := s.Pool.Exec(ctx, `INSERT INTO sessions (token_hash, user_id, created_at, expires_at, last_seen, user_agent) VALUES ($1, $2, $3, $4, $3, $5)`,
		tokenHash(token), user, now, now.Add(SessionTTL), userAgent)
	return token, err
}

// Session returns the user a session token belongs to. A session in use is
// extended, at most once a minute so reads do not each write.
func (s *Store) Session(ctx context.Context, token string) (*User, error) {
	if token == "" {
		return nil, ErrNoSession
	}
	h := tokenHash(token)
	var lastSeen time.Time
	var u User
	err := s.Pool.QueryRow(ctx, `SELECT u.id, u.email, u.name, u.role, u.disabled, u.created_at, u.last_login, s.last_seen
		FROM sessions s JOIN users u ON u.id = s.user_id WHERE s.token_hash = $1 AND s.expires_at > $2 AND NOT u.disabled`, h, s.now()).
		Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.Disabled, &u.CreatedAt, &u.LastLogin, &lastSeen)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNoSession
	}
	if err != nil {
		return nil, err
	}
	if now := s.now(); now.Sub(lastSeen) > time.Minute {
		_, _ = s.Pool.Exec(ctx, `UPDATE sessions SET last_seen = $2, expires_at = $3 WHERE token_hash = $1`, h, now, now.Add(SessionTTL))
	}
	return &u, nil
}

// Logout ends a session.
func (s *Store) Logout(ctx context.Context, token string) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM sessions WHERE token_hash = $1`, tokenHash(token))
	return err
}

// Prune deletes expired sessions.
func (s *Store) Prune(ctx context.Context) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM sessions WHERE expires_at <= $1`, s.now())
	return err
}

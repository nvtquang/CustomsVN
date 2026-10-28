package auth

import (
	"strings"
	"testing"
)

func TestPasswordHash(t *testing.T) {
	Iterations = 1000
	h, err := HashPassword("đúng mật khẩu 123")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "pbkdf2-sha256$1000$") {
		t.Fatalf("hash %q", h)
	}
	if !CheckPassword(h, "đúng mật khẩu 123") {
		t.Error("the right password fails")
	}
	for _, bad := range []string{"", "đúng mật khẩu 124", "Đúng mật khẩu 123"} {
		if CheckPassword(h, bad) {
			t.Errorf("%q passes", bad)
		}
	}
	h2, _ := HashPassword("đúng mật khẩu 123")
	if h == h2 {
		t.Error("two hashes of one password are equal: no salt")
	}
	for _, broken := range []string{"", "plain", "pbkdf2-sha256$x$a$b", "bcrypt$1$a$b", "pbkdf2-sha256$0$AAAA$AAAA"} {
		if CheckPassword(broken, "") {
			t.Errorf("%q accepted", broken)
		}
	}
}

func TestValidation(t *testing.T) {
	for email, ok := range map[string]bool{"admin": true, "a@b.vn": true, "chuyen.gia@customs.gov.vn": true,
		"": false, "no-at": false, "Tên <a@b.vn>": false, "a b@c.vn": false} {
		if got := validEmail(NormEmail(email)) == nil; got != ok {
			t.Errorf("email %q valid %v, want %v", email, got, ok)
		}
	}
	if validPassword("ngắn") == nil || validPassword("đủ mười ký tự") != nil {
		t.Error("password length")
	}
	if NormEmail("  Admin@Example.COM ") != "admin@example.com" {
		t.Error("NormEmail")
	}
}

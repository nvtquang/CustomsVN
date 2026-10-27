package secret

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestSealOpen(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))
	b, err := New(key)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := b.Seal("sk-ant-secret-1234")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(sealed), "secret") {
		t.Error("ciphertext carries the plaintext")
	}
	got, err := b.Open(sealed)
	if err != nil || got != "sk-ant-secret-1234" {
		t.Errorf("%q %v", got, err)
	}
	other, _ := New(base64.StdEncoding.EncodeToString([]byte(strings.Repeat("x", 32))))
	if _, err := other.Open(sealed); err == nil {
		t.Error("opened with the wrong key")
	}
}

func TestBadKey(t *testing.T) {
	if _, err := New("short"); err == nil {
		t.Error("accepted a short key")
	}
	t.Setenv(Env, "")
	if _, err := FromEnv(); err != ErrNoMasterKey {
		t.Errorf("got %v", err)
	}
}

func TestHint(t *testing.T) {
	if Hint("sk-ant-api03-abcdefgh1234") != "sk-…1234" || Hint("abc") != "…" {
		t.Error(Hint("sk-ant-api03-abcdefgh1234"))
	}
}

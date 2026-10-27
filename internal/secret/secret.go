// Package secret encrypts API keys at rest with AES-256-GCM under a master key
// from the environment. The master key never touches the database, so a dump
// of the database alone does not reveal a provider key.
package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
)

// Env is the variable that holds the master key: 32 bytes, base64 or hex.
const Env = "CUSTOMSVN_MASTER_KEY"

// ErrNoMasterKey means keys cannot be stored or read from the database.
var ErrNoMasterKey = errors.New(Env + " is not set: API keys can only come from environment variables")

// Box seals and opens secrets.
type Box struct{ aead cipher.AEAD }

// FromEnv returns a box for the master key in the environment, or
// ErrNoMasterKey.
func FromEnv() (*Box, error) {
	v := strings.TrimSpace(os.Getenv(Env))
	if v == "" {
		return nil, ErrNoMasterKey
	}
	return New(v)
}

// New returns a box for a base64 or hex encoded 32-byte key.
func New(encoded string) (*Box, error) {
	key, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(key) != 32 {
		key, err = hex.DecodeString(encoded)
	}
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("%s must be 32 bytes, base64 or hex (e.g. openssl rand -base64 32)", Env)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

// Seal encrypts plaintext; the nonce is stored in front of the ciphertext.
func (b *Box) Seal(plaintext string) ([]byte, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return b.aead.Seal(nonce, nonce, []byte(plaintext), nil), nil
}

// Open decrypts what Seal produced.
func (b *Box) Open(sealed []byte) (string, error) {
	n := b.aead.NonceSize()
	if len(sealed) < n {
		return "", errors.New("sealed secret is too short")
	}
	out, err := b.aead.Open(nil, sealed[:n], sealed[n:], nil)
	if err != nil {
		return "", errors.New("cannot decrypt the stored key: was the master key changed?")
	}
	return string(out), nil
}

// Hint shows enough of a key to tell two apart and no more: "sk-…a1b2".
func Hint(key string) string {
	if len(key) <= 8 {
		return "…"
	}
	prefix := key[:3]
	return prefix + "…" + key[len(key)-4:]
}

package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

// GeneratePKCE returns a PKCE code_verifier and S256 code_challenge.
func GeneratePKCE() (verifier, challenge string, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", fmt.Errorf("generate pkce verifier: %w", err)
	}
	verifier = base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

// GenerateState returns a CSRF state value for the OAuth authorize request.
func GenerateState() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate oauth state: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

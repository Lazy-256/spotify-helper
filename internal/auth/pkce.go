// Package auth implements the OAuth 2.0 Authorization Code flow with PKCE,
// the token store and transparent token refresh.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"strings"
)

// Scopes are requested at login; see the auth spec.
var Scopes = []string{
	"user-read-playback-state",
	"user-modify-playback-state",
	"user-read-currently-playing",
	"playlist-read-private",
	"user-library-read",
}

var b64 = base64.RawURLEncoding

// NewVerifier returns a PKCE code verifier: 64 random bytes, base64url without
// padding (86 characters, within the 43–128 allowed by RFC 7636).
func NewVerifier() (string, error) { return randomString(64) }

// NewState returns an unguessable value binding the callback to this login.
func NewState() (string, error) { return randomString(24) }

func randomString(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return b64.EncodeToString(buf), nil
}

// Challenge derives the S256 code challenge from a verifier.
func Challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return b64.EncodeToString(sum[:])
}

// AuthorizeURL builds the URL the user opens in the browser.
func AuthorizeURL(accountsBase, clientID, redirectURI, challenge, state string) string {
	q := url.Values{}
	q.Set("client_id", clientID)
	q.Set("response_type", "code")
	q.Set("redirect_uri", redirectURI)
	q.Set("code_challenge_method", "S256")
	q.Set("code_challenge", challenge)
	q.Set("state", state)
	q.Set("scope", strings.Join(Scopes, " "))
	return strings.TrimRight(accountsBase, "/") + "/authorize?" + q.Encode()
}

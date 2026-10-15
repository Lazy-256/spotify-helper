package auth

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Lazy-256/spotify-helper/internal/fileutil"
	"github.com/Lazy-256/spotify-helper/internal/output"
)

// Token is what login stores and refresh updates.
type Token struct {
	AccessToken  string   `json:"access_token"`
	RefreshToken string   `json:"refresh_token"`
	ExpiresAt    int64    `json:"expires_at"` // Unix seconds
	Scopes       []string `json:"scopes"`
	// ClientID is the application the token was issued to; a different
	// configured client ID makes the token unusable.
	ClientID string `json:"client_id"`
}

// Store persists the token in a private file.
type Store struct {
	Path string
}

const (
	fileMode = 0o600
	dirMode  = 0o700
)

// Read returns the stored token, nil when none exists, or an error when the
// file is unreadable or corrupt. It applies no policy; see Load.
func (s *Store) Read() (*Token, error) {
	data, err := os.ReadFile(s.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var t Token
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

// Load returns the token for clientID or a not_authenticated error telling the
// user to log in.
func (s *Store) Load(clientID string) (*Token, error) {
	t, err := s.Read()
	switch {
	case err != nil:
		return nil, output.Errorf(output.CodeNotAuthenticated, "stored token is unreadable (%v); run `spotify-helper login`", describe(err))
	case t == nil || t.RefreshToken == "" && t.AccessToken == "":
		return nil, output.Errorf(output.CodeNotAuthenticated, "not logged in; run `spotify-helper login`")
	case t.ClientID != clientID:
		return nil, output.Errorf(output.CodeNotAuthenticated, "the stored token belongs to a different client_id; run `spotify-helper login`")
	}
	return t, nil
}

// Save writes the token atomically with mode 0600 in a 0700 directory.
func (s *Store) Save(t *Token) error {
	data, err := json.Marshal(t)
	if err != nil {
		return err
	}
	if err := fileutil.WriteAtomic(s.Path, append(data, '\n'), fileMode, dirMode); err != nil {
		return err
	}
	// The directory may predate us with looser permissions.
	return os.Chmod(filepath.Dir(s.Path), dirMode)
}

// Delete removes the token; a missing file is not an error.
func (s *Store) Delete() error {
	if err := os.Remove(s.Path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// describe keeps filesystem paths out of messages while staying useful.
func describe(err error) string {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Err.Error()
	}
	return err.Error()
}

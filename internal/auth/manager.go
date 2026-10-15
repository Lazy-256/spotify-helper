package auth

import (
	"context"
	"time"

	"github.com/Lazy-256/spotify-helper/internal/fileutil"
	"github.com/Lazy-256/spotify-helper/internal/output"
)

// RefreshMargin is how long before expiry a token counts as stale.
const RefreshMargin = 60 * time.Second

// Manager hands out valid access tokens, refreshing them when needed.
type Manager struct {
	Store    *Store
	Endpoint *Endpoint
	// LockPath serialises refreshes across processes.
	LockPath string
}

// AccessToken returns an access token valid for at least RefreshMargin.
func (m *Manager) AccessToken(ctx context.Context) (string, error) {
	t, err := m.Store.Load(m.Endpoint.ClientID)
	if err != nil {
		return "", err
	}
	if m.fresh(t) {
		return t.AccessToken, nil
	}

	unlock, err := fileutil.Lock(m.LockPath, dirMode)
	if err != nil {
		return "", output.Errorf(output.CodeInternal, "cannot lock the token file: %v", describe(err))
	}
	defer unlock()

	// Another process may have refreshed while we waited for the lock;
	// refreshing again would rotate the refresh token twice.
	t, err = m.Store.Load(m.Endpoint.ClientID)
	if err != nil {
		return "", err
	}
	if m.fresh(t) {
		return t.AccessToken, nil
	}

	nt, err := m.Endpoint.Refresh(ctx, t)
	if err != nil {
		return "", err
	}
	if err := m.Store.Save(nt); err != nil {
		return "", output.Errorf(output.CodeInternal, "cannot store the refreshed token: %v", describe(err))
	}
	return nt.AccessToken, nil
}

func (m *Manager) fresh(t *Token) bool {
	return t.AccessToken != "" && t.ExpiresAt-m.Endpoint.Now().Unix() > int64(RefreshMargin/time.Second)
}

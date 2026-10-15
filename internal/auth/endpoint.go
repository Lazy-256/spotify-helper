package auth

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/Lazy-256/spotify-helper/internal/output"
	"github.com/Lazy-256/spotify-helper/internal/ratelimit"
)

// Endpoint talks to Spotify's token endpoint.
type Endpoint struct {
	HTTP         *http.Client
	AccountsBase string
	ClientID     string
	Now          func() time.Time
	// Gate is checked before and updated after every request; may be nil.
	Gate *ratelimit.Gate
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	Scope        string `json:"scope"`
}

// Exchange trades an authorization code for tokens.
func (e *Endpoint) Exchange(ctx context.Context, code, verifier, redirectURI string) (*Token, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("client_id", e.ClientID)
	form.Set("code_verifier", verifier)
	resp, err := e.post(ctx, form)
	if err != nil {
		return nil, err
	}
	if resp.AccessToken == "" || resp.RefreshToken == "" {
		return nil, output.Errorf(output.CodeAPIError, "Spotify's token response lacked a token")
	}
	return e.token(resp, nil), nil
}

// Refresh obtains a new access token. When Spotify sends no new refresh token
// (or no scope) the previous values are kept.
func (e *Endpoint) Refresh(ctx context.Context, old *Token) (*Token, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", old.RefreshToken)
	form.Set("client_id", e.ClientID)
	resp, err := e.post(ctx, form)
	if err != nil {
		return nil, err
	}
	if resp.AccessToken == "" {
		return nil, output.Errorf(output.CodeAPIError, "Spotify's token response lacked an access token")
	}
	return e.token(resp, old), nil
}

func (e *Endpoint) token(r tokenResponse, old *Token) *Token {
	t := &Token{
		AccessToken:  r.AccessToken,
		RefreshToken: r.RefreshToken,
		ExpiresAt:    e.Now().Unix() + r.ExpiresIn,
		Scopes:       strings.Fields(r.Scope),
		ClientID:     e.ClientID,
	}
	if old != nil {
		if t.RefreshToken == "" {
			t.RefreshToken = old.RefreshToken
		}
		if len(t.Scopes) == 0 {
			t.Scopes = old.Scopes
		}
	}
	return t
}

var safeOAuthError = regexp.MustCompile(`^[a-z_]{1,40}$`)

func (e *Endpoint) post(ctx context.Context, form url.Values) (tokenResponse, error) {
	var out tokenResponse
	if e.Gate != nil {
		if err := e.Gate.Check(); err != nil {
			return out, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(e.AccountsBase, "/")+"/api/token", strings.NewReader(form.Encode()))
	if err != nil {
		return out, output.Wrap(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := e.HTTP.Do(req)
	if err != nil {
		return out, output.Network(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return out, output.Network(err)
	}

	switch {
	case resp.StatusCode == http.StatusOK:
		if err := json.Unmarshal(body, &out); err != nil {
			return out, output.Errorf(output.CodeAPIError, "unreadable token response from Spotify")
		}
		return out, nil
	case resp.StatusCode == http.StatusTooManyRequests:
		if e.Gate != nil {
			return out, e.Gate.Record(resp.Header.Get("Retry-After"))
		}
		return out, output.RateLimited(ratelimit.ParseRetryAfter(resp.Header.Get("Retry-After")))
	case resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusUnauthorized:
		// invalid_grant, invalid_client, invalid_request, …: the grant or the
		// application is no good, so the user has to log in again.
		var oe struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(body, &oe)
		name := oe.Error
		if !safeOAuthError.MatchString(name) {
			name = "rejected"
		}
		return out, output.Errorf(output.CodeNotAuthenticated, "Spotify rejected the authorization (%s); run `spotify-helper login`", name)
	default:
		return out, output.Errorf(output.CodeAPIError, "Spotify token endpoint answered HTTP %d", resp.StatusCode)
	}
}

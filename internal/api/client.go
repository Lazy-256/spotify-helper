// Package api is the Spotify Web API client. It maps Spotify's wire format to
// the output types and Spotify's failures to the error contract.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Lazy-256/spotify-helper/internal/output"
	"github.com/Lazy-256/spotify-helper/internal/ratelimit"
)

// TokenSource supplies a valid access token (see auth.Manager).
type TokenSource interface {
	AccessToken(ctx context.Context) (string, error)
}

// Client calls api.spotify.com.
type Client struct {
	HTTP   *http.Client
	Base   string // e.g. https://api.spotify.com/v1
	Tokens TokenSource
	Gate   *ratelimit.Gate
	Now    func() time.Time
}

const maxResponse = 8 << 20

// do performs one request and decodes a JSON answer into out (when non-nil).
// It returns the HTTP status so callers can tell 204 (no content) from 200.
// Exactly one request is sent: 429 and other failures are never retried.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, out any) (int, error) {
	if c.Gate != nil {
		if err := c.Gate.Check(); err != nil {
			return 0, err
		}
	}
	token, err := c.Tokens.AccessToken(ctx)
	if err != nil {
		return 0, err
	}

	target := strings.TrimRight(c.Base, "/") + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, output.Wrap(err)
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return 0, output.Wrap(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, output.Network(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse))
	if err != nil {
		return 0, output.Network(err)
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if out != nil && resp.StatusCode != http.StatusNoContent && len(bytes.TrimSpace(data)) > 0 {
			if err := json.Unmarshal(data, out); err != nil {
				return resp.StatusCode, output.Errorf(output.CodeAPIError, "unreadable answer from Spotify (HTTP %d)", resp.StatusCode)
			}
		}
		return resp.StatusCode, nil
	}
	return resp.StatusCode, c.failure(resp, data)
}

// failure maps a non-2xx answer to the error contract.
func (c *Client) failure(resp *http.Response, data []byte) error {
	if resp.StatusCode == http.StatusTooManyRequests {
		if c.Gate != nil {
			return c.Gate.Record(resp.Header.Get("Retry-After"))
		}
		return output.RateLimited(ratelimit.ParseRetryAfter(resp.Header.Get("Retry-After")))
	}

	reason, message := parseError(data)
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return output.Errorf(output.CodeNotAuthenticated, "Spotify rejected the access token; run `spotify-helper login`")
	case reason == "NO_ACTIVE_DEVICE":
		return output.Errorf(output.CodeNoActiveDevice, "no active Spotify device; start playback on a device or pass --device")
	case reason == "PREMIUM_REQUIRED":
		return output.Errorf(output.CodePremiumRequired, "Spotify Premium is required for this command")
	}
	if message == "" {
		return output.Errorf(output.CodeAPIError, "Spotify answered HTTP %d", resp.StatusCode)
	}
	return output.Errorf(output.CodeAPIError, "Spotify answered HTTP %d: %s", resp.StatusCode, message)
}

// parseError extracts Spotify's error.reason and error.message. The error
// field is an object for Web API errors and a string for OAuth-style ones.
func parseError(data []byte) (reason, message string) {
	var doc struct {
		Error json.RawMessage `json:"error"`
	}
	if json.Unmarshal(data, &doc) != nil || len(doc.Error) == 0 {
		return "", ""
	}
	var obj struct {
		Reason  string `json:"reason"`
		Message string `json:"message"`
	}
	if json.Unmarshal(doc.Error, &obj) == nil {
		return obj.Reason, truncate(obj.Message, 200)
	}
	var s string
	if json.Unmarshal(doc.Error, &s) == nil {
		return "", truncate(s, 200)
	}
	return "", ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

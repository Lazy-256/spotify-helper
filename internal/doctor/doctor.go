// Package doctor builds the offline self-check report. It reads local state
// only: no network, no token refresh, and no secrets in the report.
package doctor

import (
	"slices"

	"github.com/Lazy-256/spotify-helper/internal/auth"
	"github.com/Lazy-256/spotify-helper/internal/config"
)

// Problem identifiers reported in Report.Problems.
const (
	ProblemNotConfigured    = "not_configured"
	ProblemNotAuthenticated = "not_authenticated"
	ProblemScopesMissing    = "scopes_missing"
	ProblemRateLimited      = "rate_limited"
)

// Report is the JSON document printed by `doctor`.
type Report struct {
	OK        bool       `json:"ok"`
	Problems  []string   `json:"problems"`
	Config    ConfigInfo `json:"config"`
	Token     TokenInfo  `json:"token"`
	RateLimit RateInfo   `json:"rate_limit"`
	Paths     PathsInfo  `json:"paths"`
}

type ConfigInfo struct {
	ClientIDSet    bool    `json:"client_id_set"`
	ClientIDSource *string `json:"client_id_source"`
	RedirectURI    string  `json:"redirect_uri"`
}

type TokenInfo struct {
	Present         bool     `json:"present"`
	ClientIDMatches *bool    `json:"client_id_matches"`
	HasRefreshToken bool     `json:"has_refresh_token"`
	ExpiresInS      *int64   `json:"expires_in_s"`
	Scopes          []string `json:"scopes"`
}

type RateInfo struct {
	Active     bool `json:"active"`
	RetryAfter int  `json:"retry_after"`
}

type PathsInfo struct {
	Config string `json:"config"`
	Token  string `json:"token"`
	Cache  string `json:"cache"`
}

// Inputs is everything the report is built from.
type Inputs struct {
	Paths config.Paths
	// Config is whatever Load returned; ConfigErr its error (broken file).
	Config    config.Config
	ConfigErr error
	// Token is nil when no usable token file exists (TokenErr says why when
	// the file exists but cannot be read).
	Token    *auth.Token
	TokenErr error
	// RateLimitRemaining is the seconds left of an active cooldown, else 0.
	RateLimitRemaining int
	NowUnix            int64
}

// Build assembles the report.
func Build(in Inputs) Report {
	r := Report{Problems: []string{}, Paths: PathsInfo{
		Config: in.Paths.ConfigFile(), Token: in.Paths.TokenFile(), Cache: in.Paths.CacheDir,
	}}

	r.Config = ConfigInfo{ClientIDSet: in.Config.ClientID != "", RedirectURI: in.Config.RedirectURI}
	if in.Config.ClientIDSource != config.SourceNone {
		src := in.Config.ClientIDSource
		r.Config.ClientIDSource = &src
	}
	if in.ConfigErr != nil || in.Config.ClientID == "" {
		r.Problems = append(r.Problems, ProblemNotConfigured)
	}

	r.Token.Scopes = []string{}
	switch t := in.Token; {
	case t == nil || in.TokenErr != nil || (t.AccessToken == "" && t.RefreshToken == ""):
		r.Problems = append(r.Problems, ProblemNotAuthenticated)
	default:
		r.Token.Present = true
		r.Token.HasRefreshToken = t.RefreshToken != ""
		left := t.ExpiresAt - in.NowUnix
		r.Token.ExpiresInS = &left
		if t.Scopes != nil {
			r.Token.Scopes = t.Scopes
		}
		if in.Config.ClientID != "" {
			match := t.ClientID == in.Config.ClientID
			r.Token.ClientIDMatches = &match
			if !match {
				r.Problems = append(r.Problems, ProblemNotAuthenticated)
			}
		}
		for _, s := range auth.RequiredScopes() {
			if !slices.Contains(t.Scopes, s) {
				r.Problems = append(r.Problems, ProblemScopesMissing)
				break
			}
		}
	}

	if in.RateLimitRemaining > 0 {
		r.RateLimit = RateInfo{Active: true, RetryAfter: in.RateLimitRemaining}
		r.Problems = append(r.Problems, ProblemRateLimited)
	}
	r.OK = len(r.Problems) == 0
	return r
}

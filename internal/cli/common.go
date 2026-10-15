package cli

import (
	"github.com/Lazy-256/spotify-helper/internal/auth"
	"github.com/Lazy-256/spotify-helper/internal/config"
	"github.com/Lazy-256/spotify-helper/internal/ratelimit"
)

// paths resolves the config and cache directories.
func (e *Env) paths() (config.Paths, error) {
	return config.ResolvePaths(e.Getenv)
}

// loadConfig returns paths and a configuration that has a client ID.
func (e *Env) loadConfig() (config.Paths, config.Config, error) {
	p, err := e.paths()
	if err != nil {
		return p, config.Config{}, err
	}
	cfg, err := config.Load(p, e.Getenv)
	if err != nil {
		return p, cfg, err
	}
	return p, cfg, cfg.Require()
}

func (e *Env) gate(p config.Paths) *ratelimit.Gate {
	return &ratelimit.Gate{Path: p.RateLimitFile(), Now: e.Now}
}

func (e *Env) endpoint(p config.Paths, cfg config.Config) *auth.Endpoint {
	return &auth.Endpoint{
		HTTP:         e.HTTP,
		AccountsBase: e.AccountsBase,
		ClientID:     cfg.ClientID,
		Now:          e.Now,
		Gate:         e.gate(p),
	}
}

func (e *Env) store(p config.Paths) *auth.Store {
	return &auth.Store{Path: p.TokenFile()}
}

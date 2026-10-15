// Package config resolves on-disk locations and loads the user's settings.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Lazy-256/spotify-helper/internal/output"
)

const (
	appDir = "spotify-helper"

	// EnvClientID overrides client_id from the config file.
	EnvClientID = "SPOTIFY_HELPER_CLIENT_ID"

	DefaultRedirectURI = "http://127.0.0.1:8888/callback"
)

// Client ID sources reported by Config.ClientIDSource.
const (
	SourceNone = ""
	SourceEnv  = "env"
	SourceFile = "file"
)

// Paths are the directories the helper reads and writes.
type Paths struct {
	ConfigDir string
	CacheDir  string
}

// ResolvePaths follows the XDG base directory rules.
func ResolvePaths(getenv func(string) string) (Paths, error) {
	base := func(xdgVar, homeSub string) (string, error) {
		if d := getenv(xdgVar); d != "" {
			return filepath.Join(d, appDir), nil
		}
		home := getenv("HOME")
		if home == "" {
			return "", output.Errorf(output.CodeInternal, "cannot locate %s: neither $%s nor $HOME is set", appDir, xdgVar)
		}
		return filepath.Join(home, homeSub, appDir), nil
	}
	cfg, err := base("XDG_CONFIG_HOME", ".config")
	if err != nil {
		return Paths{}, err
	}
	cache, err := base("XDG_CACHE_HOME", ".cache")
	if err != nil {
		return Paths{}, err
	}
	return Paths{ConfigDir: cfg, CacheDir: cache}, nil
}

func (p Paths) ConfigFile() string    { return filepath.Join(p.ConfigDir, "config.toml") }
func (p Paths) TokenFile() string     { return filepath.Join(p.ConfigDir, "token.json") }
func (p Paths) TokenLockFile() string { return filepath.Join(p.ConfigDir, "token.lock") }
func (p Paths) RateLimitFile() string { return filepath.Join(p.CacheDir, "rate-limit.json") }
func (p Paths) CoversDir() string     { return filepath.Join(p.CacheDir, "covers") }

// Config holds the user's settings.
type Config struct {
	ClientID       string
	ClientIDSource string
	RedirectURI    string
}

// Load reads the config file and environment. A missing file is not an error;
// callers decide whether an empty ClientID matters (see Require). The returned
// Config is always usable, even alongside an error: RedirectURI is then the
// default and ClientID whatever the environment supplied.
func Load(p Paths, getenv func(string) string) (Config, error) {
	cfg := Config{RedirectURI: DefaultRedirectURI}
	var loadErr error

	data, err := os.ReadFile(p.ConfigFile())
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		loadErr = output.Errorf(output.CodeNotConfigured, "cannot read %s: %v", p.ConfigFile(), err)
	default:
		vals, perr := parse(string(data))
		if perr != nil {
			loadErr = output.Errorf(output.CodeNotConfigured, "%s: %v", p.ConfigFile(), perr)
		} else {
			if v := vals["client_id"]; v != "" {
				cfg.ClientID, cfg.ClientIDSource = v, SourceFile
			}
			if v := vals["redirect_uri"]; v != "" {
				if rerr := ValidateRedirectURI(v); rerr != nil {
					loadErr = output.Errorf(output.CodeNotConfigured, "%s: %v", p.ConfigFile(), rerr)
				} else {
					cfg.RedirectURI = v
				}
			}
		}
	}

	if v := strings.TrimSpace(getenv(EnvClientID)); v != "" {
		cfg.ClientID, cfg.ClientIDSource = v, SourceEnv
	}
	return cfg, loadErr
}

// Require returns a not_configured error when there is no client ID.
func (c Config) Require() error {
	if c.ClientID == "" {
		return output.Errorf(output.CodeNotConfigured,
			"no client_id: set it in the config file or $%s (see README)", EnvClientID)
	}
	return nil
}

// ValidateRedirectURI checks that the URI is a loopback http URL with an
// explicit port, which is what the login flow can listen on.
func ValidateRedirectURI(s string) error {
	u, err := url.Parse(s)
	if err != nil || u.Scheme != "http" {
		return fmt.Errorf("redirect_uri must be http://127.0.0.1:<port>/<path>, got %q", s)
	}
	if ip := net.ParseIP(u.Hostname()); ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("redirect_uri host must be a loopback IP such as 127.0.0.1, got %q", u.Hostname())
	}
	if p, err := strconv.Atoi(u.Port()); err != nil || p < 1 || p > 65535 {
		return fmt.Errorf("redirect_uri needs an explicit port, got %q", s)
	}
	return nil
}

// parse reads the supported TOML subset: blank lines, # comments and
// `key = "quoted string"` pairs. Anything else is an error naming the line.
func parse(src string) (map[string]string, error) {
	out := map[string]string{}
	for n, line := range strings.Split(src, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, rest, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || !validKey(key) {
			return nil, fmt.Errorf("line %d: expected key = \"value\"", n+1)
		}
		rest = strings.TrimSpace(rest)
		if !strings.HasPrefix(rest, `"`) {
			return nil, fmt.Errorf("line %d: value of %s must be a double-quoted string", n+1, key)
		}
		end := closingQuote(rest)
		if end < 0 {
			return nil, fmt.Errorf("line %d: unterminated string for %s", n+1, key)
		}
		val, err := strconv.Unquote(rest[:end+1])
		if err != nil {
			return nil, fmt.Errorf("line %d: invalid string for %s", n+1, key)
		}
		if tail := strings.TrimSpace(rest[end+1:]); tail != "" && !strings.HasPrefix(tail, "#") {
			return nil, fmt.Errorf("line %d: unexpected text after the value of %s", n+1, key)
		}
		out[key] = val
	}
	return out, nil
}

func validKey(k string) bool {
	if k == "" {
		return false
	}
	for _, r := range k {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
		default:
			return false
		}
	}
	return true
}

// closingQuote returns the index of the quote that closes the string starting
// at s[0], honouring backslash escapes, or -1.
func closingQuote(s string) int {
	for i := 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '"':
			return i
		}
	}
	return -1
}

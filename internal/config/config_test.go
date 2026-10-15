package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Lazy-256/spotify-helper/internal/output"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func writeConfig(t *testing.T, body string) Paths {
	t.Helper()
	p := Paths{ConfigDir: filepath.Join(t.TempDir(), "cfg"), CacheDir: t.TempDir()}
	if err := os.MkdirAll(p.ConfigDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.ConfigFile(), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestResolvePaths(t *testing.T) {
	p, err := ResolvePaths(env(map[string]string{"XDG_CONFIG_HOME": "/x/c", "XDG_CACHE_HOME": "/x/k"}))
	if err != nil || p.ConfigDir != "/x/c/spotify-helper" || p.CacheDir != "/x/k/spotify-helper" {
		t.Errorf("XDG vars: %+v %v", p, err)
	}
	p, err = ResolvePaths(env(map[string]string{"HOME": "/h"}))
	if err != nil || p.ConfigDir != "/h/.config/spotify-helper" || p.CacheDir != "/h/.cache/spotify-helper" {
		t.Errorf("HOME fallback: %+v %v", p, err)
	}
	if _, err = ResolvePaths(env(nil)); err == nil {
		t.Error("no HOME and no XDG vars should fail")
	}
}

func TestMissingFileGivesDefaults(t *testing.T) {
	p := Paths{ConfigDir: t.TempDir(), CacheDir: t.TempDir()}
	cfg, err := Load(p, env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ClientID != "" || cfg.RedirectURI != DefaultRedirectURI {
		t.Errorf("unexpected defaults: %+v", cfg)
	}
	var oe *output.Error
	if err := cfg.Require(); !errors.As(err, &oe) || oe.Code != output.CodeNotConfigured {
		t.Errorf("Require without id: %v", err)
	}
}

func TestClientIDFromFile(t *testing.T) {
	p := writeConfig(t, "# my app\nclient_id = \"abc123\"  # trailing comment\n\nredirect_uri = \"http://127.0.0.1:9999/cb\"\n")
	cfg, err := Load(p, env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ClientID != "abc123" || cfg.ClientIDSource != SourceFile || cfg.RedirectURI != "http://127.0.0.1:9999/cb" {
		t.Errorf("got %+v", cfg)
	}
	if err := cfg.Require(); err != nil {
		t.Error(err)
	}
}

func TestEnvOverridesFile(t *testing.T) {
	p := writeConfig(t, `client_id = "from-file"`)
	cfg, err := Load(p, env(map[string]string{EnvClientID: " from-env "}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ClientID != "from-env" || cfg.ClientIDSource != SourceEnv {
		t.Errorf("got %+v", cfg)
	}
}

func TestUnsupportedSyntaxFailsLoudly(t *testing.T) {
	for name, body := range map[string]string{
		"section":      "[spotify]\nclient_id = \"a\"",
		"bare value":   "client_id = abc",
		"number":       "client_id = 42",
		"no equals":    "client_id",
		"unterminated": `client_id = "abc`,
		"trailing":     `client_id = "abc" extra`,
		"bad key":      `client id = "abc"`,
		"bad redirect": `redirect_uri = "https://example.com/cb"`,
	} {
		p := writeConfig(t, body)
		cfg, err := Load(p, env(map[string]string{EnvClientID: "env-id"}))
		var oe *output.Error
		if !errors.As(err, &oe) || oe.Code != output.CodeNotConfigured {
			t.Errorf("%s: want not_configured, got %v", name, err)
		}
		if !strings.Contains(oe.Message, "config.toml") {
			t.Errorf("%s: message should name the file: %q", name, oe.Message)
		}
		// The returned Config stays usable (doctor relies on it).
		if cfg.ClientID != "env-id" || cfg.RedirectURI != DefaultRedirectURI {
			t.Errorf("%s: partial config %+v", name, cfg)
		}
	}
}

func TestEscapesInStrings(t *testing.T) {
	vals, err := parse(`client_id = "a\"b#c"`)
	if err != nil || vals["client_id"] != `a"b#c` {
		t.Errorf("got %q %v", vals["client_id"], err)
	}
}

func TestValidateRedirectURI(t *testing.T) {
	for _, ok := range []string{DefaultRedirectURI, "http://127.0.0.1:1/x", "http://[::1]:8888/cb"} {
		if err := ValidateRedirectURI(ok); err != nil {
			t.Errorf("%s should be valid: %v", ok, err)
		}
	}
	for _, bad := range []string{
		"http://localhost:8888/cb", "https://127.0.0.1:8888/cb", "http://127.0.0.1/cb",
		"http://127.0.0.1:0/cb", "http://10.0.0.1:8888/cb", "http://127.0.0.1:99999/cb", "::",
	} {
		if err := ValidateRedirectURI(bad); err == nil {
			t.Errorf("%s should be rejected", bad)
		}
	}
}

func TestPathFiles(t *testing.T) {
	p := Paths{ConfigDir: "/c", CacheDir: "/k"}
	for got, want := range map[string]string{
		p.ConfigFile(): "/c/config.toml", p.TokenFile(): "/c/token.json",
		p.TokenLockFile(): "/c/token.lock", p.RateLimitFile(): "/k/rate-limit.json",
		p.CoversDir(): "/k/covers",
	} {
		if got != want {
			t.Errorf("got %s, want %s", got, want)
		}
	}
}

package cli

import (
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Lazy-256/spotify-helper/internal/auth"
)

func problems(t *testing.T, rep map[string]any) []string {
	t.Helper()
	var out []string
	for _, p := range rep["problems"].([]any) {
		out = append(out, p.(string))
	}
	return out
}

func tokenPath(env *Env) string {
	return filepath.Join(env.Getenv("XDG_CONFIG_HOME"), "spotify-helper", "token.json")
}

func TestDoctorHealthy(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	rep := parse(t, runOK(t, env, out, errb, "doctor"))
	if rep["ok"] != true || len(problems(t, rep)) != 0 {
		t.Fatalf("report: %v", rep)
	}
	if !strings.Contains(out.String(), `"problems":[]`) {
		t.Errorf("problems must be an empty array: %s", out.String())
	}
	cfg := rep["config"].(map[string]any)
	if cfg["client_id_set"] != true || cfg["client_id_source"] != "file" || cfg["redirect_uri"] != "http://127.0.0.1:8888/callback" {
		t.Errorf("config: %v", cfg)
	}
	tok := rep["token"].(map[string]any)
	if tok["present"] != true || tok["client_id_matches"] != true || tok["has_refresh_token"] != true ||
		tok["expires_in_s"] != float64(3600) || len(tok["scopes"].([]any)) != len(auth.Scopes) {
		t.Errorf("token: %v", tok)
	}
	if rl := rep["rate_limit"].(map[string]any); rl["active"] != false || rl["retry_after"] != float64(0) {
		t.Errorf("rate_limit: %v", rl)
	}
	paths := rep["paths"].(map[string]any)
	if !strings.HasSuffix(paths["token"].(string), "/spotify-helper/token.json") ||
		!strings.HasSuffix(paths["config"].(string), "/spotify-helper/config.toml") ||
		!strings.HasSuffix(paths["cache"].(string), "/spotify-helper") {
		t.Errorf("paths: %v", paths)
	}
	if f.total() != 0 {
		t.Errorf("doctor sent %d requests", f.total())
	}
}

func TestDoctorNothingSetUp(t *testing.T) {
	env, out, errb := testEnv(t)
	rep := parse(t, runOK(t, env, out, errb, "doctor")) // exit 0 even though broken
	if rep["ok"] != false {
		t.Errorf("ok: %v", rep["ok"])
	}
	got := problems(t, rep)
	if !slices.Contains(got, "not_configured") || !slices.Contains(got, "not_authenticated") {
		t.Errorf("problems: %v", got)
	}
	cfg := rep["config"].(map[string]any)
	if cfg["client_id_set"] != false || cfg["client_id_source"] != nil {
		t.Errorf("config: %v", cfg)
	}
	tok := rep["token"].(map[string]any)
	if tok["present"] != false || tok["client_id_matches"] != nil || tok["expires_in_s"] != nil || len(tok["scopes"].([]any)) != 0 {
		t.Errorf("token: %v", tok)
	}
}

func TestDoctorBrokenConfigStillRuns(t *testing.T) {
	env, out, errb := testEnv(t)
	dir := filepath.Join(env.Getenv("XDG_CONFIG_HOME"), "spotify-helper")
	_ = os.MkdirAll(dir, 0o700)
	_ = os.WriteFile(filepath.Join(dir, "config.toml"), []byte("[section]\n"), 0o600)
	rep := parse(t, runOK(t, env, out, errb, "doctor"))
	if !slices.Contains(problems(t, rep), "not_configured") {
		t.Errorf("problems: %v", rep["problems"])
	}
}

func TestDoctorCorruptToken(t *testing.T) {
	env, _, out, errb := spotifyEnv(t)
	_ = os.WriteFile(tokenPath(env), []byte("{broken"), 0o600)
	rep := parse(t, runOK(t, env, out, errb, "doctor"))
	if !slices.Contains(problems(t, rep), "not_authenticated") || rep["token"].(map[string]any)["present"] != false {
		t.Errorf("report: %v", rep)
	}
}

func TestDoctorTokenForAnotherClient(t *testing.T) {
	env, _, out, errb := spotifyEnv(t)
	store := &auth.Store{Path: tokenPath(env)}
	_ = store.Save(&auth.Token{AccessToken: "A", RefreshToken: "R", ExpiresAt: env.Now().Unix() + 100, ClientID: "other", Scopes: auth.Scopes})
	rep := parse(t, runOK(t, env, out, errb, "doctor"))
	tok := rep["token"].(map[string]any)
	if tok["client_id_matches"] != false || !slices.Contains(problems(t, rep), "not_authenticated") {
		t.Errorf("report: %v", rep)
	}
}

func TestDoctorMissingScope(t *testing.T) {
	env, _, out, errb := spotifyEnv(t)
	store := &auth.Store{Path: tokenPath(env)}
	_ = store.Save(&auth.Token{AccessToken: "A", RefreshToken: "R", ExpiresAt: env.Now().Unix() + 100,
		ClientID: "test-client-id", Scopes: auth.Scopes[:len(auth.Scopes)-1]})
	rep := parse(t, runOK(t, env, out, errb, "doctor"))
	if got := problems(t, rep); !slices.Equal(got, []string{"scopes_missing"}) {
		t.Errorf("problems: %v", got)
	}
}

func TestDoctorRateLimited(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.on("GET /me/player", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "20")
		w.WriteHeader(429)
	})
	runFail(t, env, out, errb, 5, "rate_limited", "state")
	f.on("GET /me/player", nil)

	before := f.total()
	rep := parse(t, runOK(t, env, out, errb, "doctor"))
	rl := rep["rate_limit"].(map[string]any)
	if rl["active"] != true || rl["retry_after"].(float64) < 1 || !slices.Contains(problems(t, rep), "rate_limited") {
		t.Errorf("report: %v", rep)
	}
	if f.total() != before {
		t.Error("doctor must not contact Spotify")
	}
}

func TestDoctorIsOfflineAndLeavesTheTokenAlone(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	store := &auth.Store{Path: tokenPath(env)}
	_ = store.Save(&auth.Token{AccessToken: "OLD", RefreshToken: "R", ExpiresAt: env.Now().Unix() - 500,
		ClientID: "test-client-id", Scopes: auth.Scopes})
	before, _ := os.ReadFile(tokenPath(env))

	rep := parse(t, runOK(t, env, out, errb, "doctor"))
	if exp := rep["token"].(map[string]any)["expires_in_s"].(float64); exp != -500 {
		t.Errorf("expires_in_s %v, want -500 for an expired token", exp)
	}
	after, _ := os.ReadFile(tokenPath(env))
	if string(before) != string(after) {
		t.Error("doctor must not refresh or rewrite the token")
	}
	if f.total() != 0 {
		t.Errorf("%d requests", f.total())
	}
}

func TestDoctorNeverPrintsSecrets(t *testing.T) {
	env, _, out, errb := spotifyEnv(t)
	runOK(t, env, out, errb, "doctor")
	all := out.String() + errb.String()
	for _, secret := range []string{"test-client-id", "TOKEN-VALUE", "REFRESH-VALUE"} {
		if strings.Contains(all, secret) {
			t.Errorf("doctor output contains %q", secret)
		}
	}

	// ...also when the client ID comes from the environment.
	orig := env.Getenv
	env.Getenv = func(k string) string {
		if k == "SPOTIFY_HELPER_CLIENT_ID" {
			return "env-client-id"
		}
		return orig(k)
	}
	rep := parse(t, runOK(t, env, out, errb, "doctor"))
	if strings.Contains(out.String(), "env-client-id") || rep["config"].(map[string]any)["client_id_source"] != "env" {
		t.Errorf("env source: %s", out.String())
	}
}

// A token issued before user-library-modify existed has every older scope.
func TestDoctorTokenFromBeforeLibraryModify(t *testing.T) {
	env, _, out, errb := spotifyEnv(t)
	old := []string{"user-read-playback-state", "user-modify-playback-state", "user-read-currently-playing",
		"playlist-read-private", "user-library-read"}
	store := &auth.Store{Path: tokenPath(env)}
	_ = store.Save(&auth.Token{AccessToken: "A", RefreshToken: "R", ExpiresAt: env.Now().Unix() + 100,
		ClientID: "test-client-id", Scopes: old})
	rep := parse(t, runOK(t, env, out, errb, "doctor"))
	if got := problems(t, rep); !slices.Equal(got, []string{"scopes_missing"}) {
		t.Errorf("problems: %v", got)
	}
}

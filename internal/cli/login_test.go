package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// loginEnv prepares an Env with a configured client ID, a free loopback
// redirect port and a fake accounts server. browser decides what the "user"
// does with the authorization URL.
func loginEnv(t *testing.T, browser func(t *testing.T, authURL *url.URL)) (*Env, *bytes.Buffer, *bytes.Buffer, string) {
	t.Helper()
	env, out, errb := testEnv(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	redirect := fmt.Sprintf("http://%s/callback", ln.Addr())
	ln.Close()

	cfgDir := filepath.Join(env.Getenv("XDG_CONFIG_HOME"), "spotify-helper")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf("client_id = \"test-client-id\"\nredirect_uri = %q\n", redirect)
	if err := os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	accounts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.URL.Path != "/api/token" || r.PostForm.Get("code") != "THE-CODE" || r.PostForm.Get("code_verifier") == "" {
			w.WriteHeader(400)
			_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
			return
		}
		_, _ = w.Write([]byte(`{"access_token":"AT","refresh_token":"RT","expires_in":3600,"scope":"user-read-playback-state user-library-read"}`))
	}))
	t.Cleanup(accounts.Close)

	env.HTTP = accounts.Client()
	env.AccountsBase = accounts.URL
	env.LoginTimeout = 5 * time.Second
	env.OpenBrowser = func(raw string) error {
		u, err := url.Parse(raw)
		if err != nil {
			t.Error(err)
			return err
		}
		go browser(t, u)
		return nil
	}
	return env, out, errb, filepath.Join(cfgDir, "token.json")
}

// approve behaves like a user who clicks "Agree".
func approve(t *testing.T, authURL *url.URL) {
	q := authURL.Query()
	if q.Get("client_id") != "test-client-id" || q.Get("code_challenge_method") != "S256" {
		t.Errorf("unexpected authorize URL %s", authURL)
	}
	resp, err := http.Get(q.Get("redirect_uri") + "?code=THE-CODE&state=" + url.QueryEscape(q.Get("state")))
	if err != nil {
		t.Error(err)
		return
	}
	resp.Body.Close()
}

func TestLoginEndToEnd(t *testing.T) {
	env, out, errb, tokenPath := loginEnv(t, approve)
	if code := Run([]string{"login"}, env); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errb.String())
	}

	// stdout is exactly one JSON document.
	if strings.Count(out.String(), "\n") != 1 {
		t.Fatalf("stdout must be one line, got %q", out.String())
	}
	var doc struct {
		OK     bool     `json:"ok"`
		Scopes []string `json:"scopes"`
	}
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil || !doc.OK || len(doc.Scopes) != 2 {
		t.Errorf("stdout %q (%v)", out.String(), err)
	}

	// The URL went to stderr, and tokens did not leak anywhere.
	if !strings.Contains(errb.String(), "https://") && !strings.Contains(errb.String(), "http://") {
		t.Errorf("stderr should show the authorization URL, got %q", errb.String())
	}
	for _, secret := range []string{"AT", "RT", "THE-CODE"} {
		if strings.Contains(out.String(), `"`+secret+`"`) || strings.Contains(errb.String(), secret+"\n") {
			t.Errorf("secret %s leaked", secret)
		}
	}
	if strings.Contains(out.String(), "test-client-id") {
		t.Error("client ID must not be on stdout")
	}

	fi, err := os.Stat(tokenPath)
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("token file: %v %v", fi, err)
	}
	var stored map[string]any
	data, _ := os.ReadFile(tokenPath)
	if err := json.Unmarshal(data, &stored); err != nil || stored["access_token"] != "AT" || stored["client_id"] != "test-client-id" {
		t.Errorf("stored token %s", data)
	}
}

func TestLoginStopsListening(t *testing.T) {
	var redirect string
	env, _, _, _ := loginEnv(t, func(t *testing.T, u *url.URL) {
		redirect = u.Query().Get("redirect_uri")
		approve(t, u)
	})
	if code := Run([]string{"login"}, env); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if _, err := http.Get(redirect + "?code=x&state=y"); err == nil {
		t.Error("the callback port must be closed after login")
	}
}

func TestLoginDenied(t *testing.T) {
	env, out, errb, tokenPath := loginEnv(t, func(t *testing.T, u *url.URL) {
		q := u.Query()
		resp, err := http.Get(q.Get("redirect_uri") + "?error=access_denied&state=" + url.QueryEscape(q.Get("state")))
		if err == nil {
			resp.Body.Close()
		}
	})
	if code := Run([]string{"login"}, env); code != 3 {
		t.Fatalf("exit %d, want 3 (stderr %q)", code, errb.String())
	}
	if out.Len() != 0 {
		t.Errorf("stdout must be empty on failure: %q", out.String())
	}
	if _, err := os.Stat(tokenPath); err == nil {
		t.Error("no token may be stored after a denial")
	}
}

func TestLoginTimeout(t *testing.T) {
	env, _, errb, tokenPath := loginEnv(t, func(*testing.T, *url.URL) {})
	env.LoginTimeout = 100 * time.Millisecond
	if code := Run([]string{"login"}, env); code != 3 {
		t.Fatalf("exit %d, want 3 (stderr %q)", code, errb.String())
	}
	if got := errorCode(t, errb); got != "not_authenticated" {
		t.Errorf("code %q", got)
	}
	if _, err := os.Stat(tokenPath); err == nil {
		t.Error("no token may be stored after a timeout")
	}
}

func TestLoginPortBusy(t *testing.T) {
	env, _, errb, _ := loginEnv(t, func(*testing.T, *url.URL) {})
	// Occupy the configured port.
	cfg, _ := os.ReadFile(filepath.Join(env.Getenv("XDG_CONFIG_HOME"), "spotify-helper", "config.toml"))
	host := strings.TrimPrefix(strings.Split(strings.Split(string(cfg), `redirect_uri = "http://`)[1], "/")[0], "")
	busy, err := net.Listen("tcp", host)
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	if code := Run([]string{"login"}, env); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if got := errorCode(t, errb); got != "internal" {
		t.Errorf("code %q", got)
	}
}

func TestLoginWithoutClientID(t *testing.T) {
	env, out, errb := testEnv(t)
	if code := Run([]string{"login"}, env); code != 3 {
		t.Fatalf("exit %d", code)
	}
	if errorCode(t, errb) != "not_configured" || out.Len() != 0 {
		t.Errorf("stdout %q stderr %q", out.String(), errb.String())
	}
}

func TestLogoutIsIdempotent(t *testing.T) {
	env, out, errb, tokenPath := loginEnv(t, approve)
	if code := Run([]string{"login"}, env); code != 0 {
		t.Fatalf("login failed: %s", errb.String())
	}
	for i := range 2 {
		out.Reset()
		errb.Reset()
		if code := Run([]string{"logout"}, env); code != 0 {
			t.Fatalf("logout #%d: exit %d (%s)", i+1, code, errb.String())
		}
		if out.String() != "{\"ok\":true}\n" {
			t.Errorf("logout #%d: stdout %q", i+1, out.String())
		}
	}
	if _, err := os.Stat(tokenPath); err == nil {
		t.Error("token file should be gone")
	}
}

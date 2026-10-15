package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Lazy-256/spotify-helper/internal/auth"
)

// call is one request that reached the fake Spotify.
type call struct {
	Method string
	Path   string // without the /v1 prefix
	Query  url.Values
	Body   string
}

func (c call) json(t *testing.T) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(c.Body), &m); err != nil {
		t.Fatalf("body %q is not JSON: %v", c.Body, err)
	}
	return m
}

// fakeSpotify serves api.spotify.com. Routes are "METHOD /path" keys; an
// unknown route answers 599 so a stray request is visible in the test.
type fakeSpotify struct {
	t      *testing.T
	srv    *httptest.Server
	mu     sync.Mutex
	routes map[string]http.HandlerFunc
	calls  []call
}

func (f *fakeSpotify) on(route string, h http.HandlerFunc) {
	f.mu.Lock()
	f.routes[route] = h
	f.mu.Unlock()
}

func (f *fakeSpotify) reply(route string, status int, body string) {
	f.on(route, func(w http.ResponseWriter, r *http.Request) {
		if body != "" {
			w.Header().Set("Content-Type", "application/json")
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	})
}

func (f *fakeSpotify) callsTo(route string) []call {
	f.mu.Lock()
	defer f.mu.Unlock()
	method, path, _ := strings.Cut(route, " ")
	var out []call
	for _, c := range f.calls {
		if c.Method == method && c.Path == path {
			out = append(out, c)
		}
	}
	return out
}

func (f *fakeSpotify) total() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

// spotifyEnv returns an Env configured and logged in against a fake Spotify.
func spotifyEnv(t *testing.T) (*Env, *fakeSpotify, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	env, out, errb := testEnv(t)

	cfgDir := filepath.Join(env.Getenv("XDG_CONFIG_HOME"), "spotify-helper")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte("client_id = \"test-client-id\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := &auth.Store{Path: filepath.Join(cfgDir, "token.json")}
	if err := store.Save(&auth.Token{
		AccessToken: "TOKEN-VALUE", RefreshToken: "REFRESH-VALUE",
		ExpiresAt: env.Now().Unix() + 3600, ClientID: "test-client-id",
		Scopes: auth.Scopes,
	}); err != nil {
		t.Fatal(err)
	}

	f := &fakeSpotify{t: t, routes: map[string]http.HandlerFunc{}}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		path := strings.TrimPrefix(r.URL.Path, "/v1")
		f.mu.Lock()
		f.calls = append(f.calls, call{r.Method, path, r.URL.Query(), string(body)})
		h := f.routes[r.Method+" "+path]
		f.mu.Unlock()
		if got := r.Header.Get("Authorization"); got != "Bearer TOKEN-VALUE" {
			t.Errorf("Authorization = %q", got)
		}
		if h == nil {
			w.WriteHeader(599)
			return
		}
		h(w, r)
	}))
	t.Cleanup(f.srv.Close)
	env.HTTP = f.srv.Client()
	env.APIBase = f.srv.URL + "/v1"
	return env, f, out, errb
}

// runOK runs a command that must succeed and returns its stdout.
func runOK(t *testing.T, env *Env, out, errb *bytes.Buffer, args ...string) string {
	t.Helper()
	out.Reset()
	errb.Reset()
	if code := Run(args, env); code != 0 {
		t.Fatalf("%v: exit %d, stderr %q", args, code, errb.String())
	}
	if errb.Len() != 0 {
		t.Errorf("%v: stderr should be empty, got %q", args, errb.String())
	}
	return out.String()
}

// runFail runs a command that must fail with the given exit and error code.
func runFail(t *testing.T, env *Env, out, errb *bytes.Buffer, wantExit int, wantCode string, args ...string) {
	t.Helper()
	out.Reset()
	errb.Reset()
	code := Run(args, env)
	if code != wantExit {
		t.Fatalf("%v: exit %d, want %d (stderr %q)", args, code, wantExit, errb.String())
	}
	if out.Len() != 0 {
		t.Errorf("%v: stdout must be empty on failure, got %q", args, out.String())
	}
	if got := errorCode(t, errb); got != wantCode {
		t.Errorf("%v: error code %q, want %q", args, got, wantCode)
	}
	if strings.Count(strings.TrimRight(errb.String(), "\n"), "\n") != 0 {
		t.Errorf("%v: stderr should be a single line, got %q", args, errb.String())
	}
}

func parse(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("not a JSON object: %q (%v)", s, err)
	}
	return m
}

func removeFile(path string) error { return os.Remove(path) }

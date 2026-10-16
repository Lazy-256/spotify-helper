package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Lazy-256/spotify-helper/internal/output"
	"github.com/Lazy-256/spotify-helper/internal/ratelimit"
)

// ---- PKCE ----

func TestChallengeMatchesRFC7636Vector(t *testing.T) {
	const verifier = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	const want = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	if got := Challenge(verifier); got != want {
		t.Errorf("Challenge = %s, want %s", got, want)
	}
}

func TestVerifierShapeAndUniqueness(t *testing.T) {
	re := regexp.MustCompile(`^[A-Za-z0-9_-]{43,128}$`)
	a, err := NewVerifier()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := NewVerifier()
	if !re.MatchString(a) || len(a) != 86 {
		t.Errorf("verifier %q has the wrong shape", a)
	}
	if a == b {
		t.Error("verifiers must be random")
	}
}

func TestAuthorizeURL(t *testing.T) {
	raw := AuthorizeURL("https://accounts.example/", "CID", "http://127.0.0.1:8888/callback", "CH", "ST")
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if u.Scheme+"://"+u.Host+u.Path != "https://accounts.example/authorize" {
		t.Errorf("endpoint: %s", raw)
	}
	q := u.Query()
	want := map[string]string{
		"client_id": "CID", "response_type": "code",
		"redirect_uri":          "http://127.0.0.1:8888/callback",
		"code_challenge_method": "S256", "code_challenge": "CH", "state": "ST",
		"scope": "user-read-playback-state user-modify-playback-state user-read-currently-playing playlist-read-private user-library-read user-library-modify",
	}
	for k, v := range want {
		if q.Get(k) != v {
			t.Errorf("%s = %q, want %q", k, q.Get(k), v)
		}
	}
	if q.Has("client_secret") {
		t.Error("PKCE flow must not use a client secret")
	}
}

// ---- callback listener ----

func freeRedirect(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return fmt.Sprintf("http://%s/callback", ln.Addr())
}

func get(t *testing.T, rawurl string) int {
	t.Helper()
	resp, err := http.Get(rawurl)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

// fire is get for goroutines: it reports with Error, not Fatal.
func fire(t *testing.T, rawurl string) {
	resp, err := http.Get(rawurl)
	if err != nil {
		t.Error(err)
		return
	}
	resp.Body.Close()
}

func codeOf(err error) output.Code {
	var oe *output.Error
	if errors.As(err, &oe) {
		return oe.Code
	}
	return ""
}

func TestCallbackSuccess(t *testing.T) {
	redirect := freeRedirect(t)
	l, err := Listen(redirect, "S1")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go fire(t, redirect+"?code=CODE123&state=S1")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	code, err := l.Wait(ctx)
	if err != nil || code != "CODE123" {
		t.Fatalf("got %q %v", code, err)
	}
}

func TestCallbackDenied(t *testing.T) {
	redirect := freeRedirect(t)
	l, _ := Listen(redirect, "S1")
	defer l.Close()
	go fire(t, redirect+"?error=access_denied&state=S1")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := l.Wait(ctx)
	if codeOf(err) != output.CodeNotAuthenticated {
		t.Errorf("want not_authenticated, got %v", err)
	}
}

func TestCallbackStateMismatchKeepsWaiting(t *testing.T) {
	redirect := freeRedirect(t)
	l, _ := Listen(redirect, "GOOD")
	defer l.Close()

	if st := get(t, redirect+"?code=EVIL&state=BAD"); st != http.StatusBadRequest {
		t.Errorf("mismatched state: HTTP %d, want 400", st)
	}
	if st := get(t, redirect+"?code=EVIL"); st != http.StatusBadRequest {
		t.Errorf("missing state: HTTP %d, want 400", st)
	}
	if st := get(t, strings.Replace(redirect, "/callback", "/other", 1)+"?code=X&state=GOOD"); st != http.StatusNotFound {
		t.Errorf("wrong path: HTTP %d, want 404", st)
	}

	go fire(t, redirect+"?code=REAL&state=GOOD")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	code, err := l.Wait(ctx)
	if err != nil || code != "REAL" {
		t.Fatalf("the valid callback must still win, got %q %v", code, err)
	}
}

func TestCallbackTimeout(t *testing.T) {
	l, _ := Listen(freeRedirect(t), "S")
	defer l.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := l.Wait(ctx)
	if codeOf(err) != output.CodeNotAuthenticated {
		t.Errorf("want not_authenticated, got %v", err)
	}
}

func TestListenPortBusy(t *testing.T) {
	redirect := freeRedirect(t)
	first, err := Listen(redirect, "S")
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	_, err = Listen(redirect, "S")
	var oe *output.Error
	if !errors.As(err, &oe) || oe.Code != output.CodeInternal {
		t.Fatalf("want internal error, got %v", err)
	}
	port := redirect[strings.LastIndex(redirect, ":")+1 : strings.Index(redirect, "/callback")]
	if !strings.Contains(oe.Message, port) {
		t.Errorf("message should name port %s: %q", port, oe.Message)
	}
}

func TestListenerStopsAfterClose(t *testing.T) {
	redirect := freeRedirect(t)
	l, _ := Listen(redirect, "S")
	l.Close()
	if _, err := http.Get(redirect + "?code=a&state=S"); err == nil {
		t.Error("the port should be closed after Close")
	}
}

// ---- token endpoint ----

type fakeAccounts struct {
	*httptest.Server
	mu       sync.Mutex
	requests []url.Values
	handler  func(form url.Values, w http.ResponseWriter)
}

func newAccounts(t *testing.T, h func(form url.Values, w http.ResponseWriter)) *fakeAccounts {
	t.Helper()
	f := &fakeAccounts{handler: h}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/token" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		_ = r.ParseForm()
		f.mu.Lock()
		f.requests = append(f.requests, r.PostForm)
		f.mu.Unlock()
		f.handler(r.PostForm, w)
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeAccounts) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

var now = time.Unix(1_700_000_000, 0)

func nowFn() time.Time { return now }

func newEndpoint(f *fakeAccounts, gate *ratelimit.Gate) *Endpoint {
	return &Endpoint{HTTP: f.Client(), AccountsBase: f.URL, ClientID: "CID", Now: nowFn, Gate: gate}
}

func TestExchange(t *testing.T) {
	f := newAccounts(t, func(form url.Values, w http.ResponseWriter) {
		writeJSON(w, 200, map[string]any{
			"access_token": "AT", "refresh_token": "RT", "expires_in": 3600,
			"token_type": "Bearer", "scope": "user-read-playback-state user-library-read",
		})
	})
	tok, err := newEndpoint(f, nil).Exchange(context.Background(), "CODE", "VER", "http://127.0.0.1:8888/callback")
	if err != nil {
		t.Fatal(err)
	}
	form := f.requests[0]
	for k, v := range map[string]string{
		"grant_type": "authorization_code", "code": "CODE", "code_verifier": "VER",
		"client_id": "CID", "redirect_uri": "http://127.0.0.1:8888/callback",
	} {
		if form.Get(k) != v {
			t.Errorf("form %s = %q, want %q", k, form.Get(k), v)
		}
	}
	if form.Has("client_secret") {
		t.Error("client_secret must not be sent")
	}
	if tok.AccessToken != "AT" || tok.RefreshToken != "RT" || tok.ClientID != "CID" ||
		tok.ExpiresAt != now.Unix()+3600 || len(tok.Scopes) != 2 {
		t.Errorf("token: %+v", tok)
	}
}

func TestRefreshKeepsOldRefreshTokenAndScopes(t *testing.T) {
	f := newAccounts(t, func(form url.Values, w http.ResponseWriter) {
		if form.Get("refresh_token") != "OLD" || form.Get("grant_type") != "refresh_token" {
			writeJSON(w, 400, map[string]string{"error": "invalid_grant"})
			return
		}
		writeJSON(w, 200, map[string]any{"access_token": "NEW", "expires_in": 3600})
	})
	tok, err := newEndpoint(f, nil).Refresh(context.Background(), &Token{RefreshToken: "OLD", Scopes: []string{"a"}})
	if err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken != "NEW" || tok.RefreshToken != "OLD" || len(tok.Scopes) != 1 {
		t.Errorf("token: %+v", tok)
	}
}

func TestRefreshUsesRotatedRefreshToken(t *testing.T) {
	f := newAccounts(t, func(form url.Values, w http.ResponseWriter) {
		writeJSON(w, 200, map[string]any{"access_token": "NEW", "refresh_token": "ROTATED", "expires_in": 3600})
	})
	tok, err := newEndpoint(f, nil).Refresh(context.Background(), &Token{RefreshToken: "OLD"})
	if err != nil || tok.RefreshToken != "ROTATED" {
		t.Errorf("got %+v %v", tok, err)
	}
}

func TestRejectedGrantIsNotAuthenticatedWithoutLeakingSecrets(t *testing.T) {
	f := newAccounts(t, func(form url.Values, w http.ResponseWriter) {
		writeJSON(w, 400, map[string]string{"error": "invalid_grant", "error_description": "Refresh token revoked SECRET-RT"})
	})
	_, err := newEndpoint(f, nil).Refresh(context.Background(), &Token{RefreshToken: "SECRET-RT"})
	if codeOf(err) != output.CodeNotAuthenticated {
		t.Fatalf("want not_authenticated, got %v", err)
	}
	for _, secret := range []string{"SECRET-RT", "CID"} {
		if strings.Contains(err.Error(), secret) {
			t.Errorf("error message leaks %q: %v", secret, err)
		}
	}
}

func TestTokenEndpointRateLimit(t *testing.T) {
	f := newAccounts(t, func(form url.Values, w http.ResponseWriter) {
		w.Header().Set("Retry-After", "12")
		w.WriteHeader(429)
	})
	gate := &ratelimit.Gate{Path: filepath.Join(t.TempDir(), "rl.json"), Now: nowFn}
	ep := newEndpoint(f, gate)

	_, err := ep.Refresh(context.Background(), &Token{RefreshToken: "R"})
	var oe *output.Error
	if !errors.As(err, &oe) || oe.Code != output.CodeRateLimited || oe.RetryAfter != 12 {
		t.Fatalf("got %v", err)
	}
	if _, err = ep.Refresh(context.Background(), &Token{RefreshToken: "R"}); codeOf(err) != output.CodeRateLimited {
		t.Fatalf("second call: %v", err)
	}
	if n := f.count(); n != 1 {
		t.Errorf("%d requests reached Spotify, want 1 (no retry, cooldown honoured)", n)
	}
}

func TestTokenEndpointNetworkError(t *testing.T) {
	f := newAccounts(t, func(form url.Values, w http.ResponseWriter) {
		time.Sleep(300 * time.Millisecond)
	})
	ep := newEndpoint(f, nil)
	ep.HTTP = &http.Client{Timeout: 50 * time.Millisecond}
	_, err := ep.Refresh(context.Background(), &Token{RefreshToken: "R"})
	if codeOf(err) != output.CodeNetworkError {
		t.Errorf("want network_error, got %v", err)
	}
}

func TestTokenEndpointServerError(t *testing.T) {
	f := newAccounts(t, func(form url.Values, w http.ResponseWriter) { w.WriteHeader(503) })
	_, err := newEndpoint(f, nil).Refresh(context.Background(), &Token{RefreshToken: "R"})
	if codeOf(err) != output.CodeAPIError || !strings.Contains(err.Error(), "503") {
		t.Errorf("got %v", err)
	}
}

// ---- store ----

func TestStoreRoundTripAndPermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cfg")
	if err := os.MkdirAll(dir, 0o755); err != nil { // pre-existing, too open
		t.Fatal(err)
	}
	s := &Store{Path: filepath.Join(dir, "token.json")}
	want := &Token{AccessToken: "A", RefreshToken: "R", ExpiresAt: 5, Scopes: []string{"x"}, ClientID: "CID"}
	if err := s.Save(want); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(s.Path); fi.Mode().Perm() != 0o600 {
		t.Errorf("file mode %v", fi.Mode().Perm())
	}
	if fi, _ := os.Stat(dir); fi.Mode().Perm() != 0o700 {
		t.Errorf("dir mode %v", fi.Mode().Perm())
	}
	got, err := s.Load("CID")
	if err != nil || got.AccessToken != "A" || got.RefreshToken != "R" || got.ExpiresAt != 5 {
		t.Errorf("got %+v %v", got, err)
	}
}

func TestStoreLoadFailuresAreNotAuthenticated(t *testing.T) {
	dir := t.TempDir()
	s := &Store{Path: filepath.Join(dir, "token.json")}
	if _, err := s.Load("CID"); codeOf(err) != output.CodeNotAuthenticated {
		t.Errorf("missing file: %v", err)
	}
	if tok, err := s.Read(); tok != nil || err != nil {
		t.Errorf("Read of a missing file should be (nil, nil), got %v %v", tok, err)
	}

	os.WriteFile(s.Path, []byte("{broken"), 0o600)
	if _, err := s.Load("CID"); codeOf(err) != output.CodeNotAuthenticated {
		t.Errorf("corrupt file: %v", err)
	}

	_ = s.Save(&Token{AccessToken: "A", RefreshToken: "R", ClientID: "OTHER"})
	_, err := s.Load("CID")
	if codeOf(err) != output.CodeNotAuthenticated || !strings.Contains(err.Error(), "different client_id") {
		t.Errorf("client ID mismatch: %v", err)
	}
}

func TestStoreDeleteIsIdempotent(t *testing.T) {
	s := &Store{Path: filepath.Join(t.TempDir(), "token.json")}
	_ = s.Save(&Token{AccessToken: "A", RefreshToken: "R", ClientID: "C"})
	for range 2 {
		if err := s.Delete(); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(s.Path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("file should be gone: %v", err)
	}
}

// ---- manager ----

func newManager(t *testing.T, dir string, f *fakeAccounts) *Manager {
	t.Helper()
	return &Manager{
		Store:    &Store{Path: filepath.Join(dir, "token.json")},
		Endpoint: newEndpoint(f, nil),
		LockPath: filepath.Join(dir, "token.lock"),
	}
}

func TestFreshTokenNeedsNoRequest(t *testing.T) {
	f := newAccounts(t, func(url.Values, http.ResponseWriter) { t.Error("no request expected") })
	dir := t.TempDir()
	m := newManager(t, dir, f)
	_ = m.Store.Save(&Token{AccessToken: "AT", RefreshToken: "RT", ExpiresAt: now.Unix() + 3600, ClientID: "CID"})
	got, err := m.AccessToken(context.Background())
	if err != nil || got != "AT" {
		t.Errorf("got %q %v", got, err)
	}
}

func TestRefreshWithinMargin(t *testing.T) {
	f := newAccounts(t, func(form url.Values, w http.ResponseWriter) {
		writeJSON(w, 200, map[string]any{"access_token": "NEW", "expires_in": 3600})
	})
	m := newManager(t, t.TempDir(), f)
	// 30 s left is inside the 60 s margin.
	_ = m.Store.Save(&Token{AccessToken: "OLD", RefreshToken: "RT", ExpiresAt: now.Unix() + 30, ClientID: "CID"})
	got, err := m.AccessToken(context.Background())
	if err != nil || got != "NEW" {
		t.Fatalf("got %q %v", got, err)
	}
	stored, _ := m.Store.Load("CID")
	if stored.AccessToken != "NEW" || stored.RefreshToken != "RT" || stored.ExpiresAt != now.Unix()+3600 {
		t.Errorf("stored token not updated: %+v", stored)
	}
}

func TestRevokedGrant(t *testing.T) {
	f := newAccounts(t, func(form url.Values, w http.ResponseWriter) {
		writeJSON(w, 400, map[string]string{"error": "invalid_grant"})
	})
	m := newManager(t, t.TempDir(), f)
	_ = m.Store.Save(&Token{AccessToken: "OLD", RefreshToken: "RT", ExpiresAt: now.Unix() - 10, ClientID: "CID"})
	if _, err := m.AccessToken(context.Background()); codeOf(err) != output.CodeNotAuthenticated {
		t.Errorf("want not_authenticated, got %v", err)
	}
}

func TestNoTokenMeansNotAuthenticated(t *testing.T) {
	f := newAccounts(t, func(url.Values, http.ResponseWriter) { t.Error("no request expected") })
	m := newManager(t, t.TempDir(), f)
	if _, err := m.AccessToken(context.Background()); codeOf(err) != output.CodeNotAuthenticated {
		t.Errorf("got %v", err)
	}
}

// Concurrent processes that all find an expired token must refresh once: the
// fake server rotates the refresh token, so a second refresh with the old one
// would be rejected and the stored refresh token lost.
func TestConcurrentRefreshRotatesOnce(t *testing.T) {
	var current atomic.Value
	current.Store("RT0")
	var refreshes atomic.Int32
	f := newAccounts(t, func(form url.Values, w http.ResponseWriter) {
		if form.Get("refresh_token") != current.Load().(string) {
			writeJSON(w, 400, map[string]string{"error": "invalid_grant"})
			return
		}
		n := refreshes.Add(1)
		time.Sleep(30 * time.Millisecond) // widen the race window
		next := fmt.Sprintf("RT%d", n)
		current.Store(next)
		writeJSON(w, 200, map[string]any{"access_token": "AT" + next, "refresh_token": next, "expires_in": 3600})
	})
	dir := t.TempDir()
	seed := newManager(t, dir, f)
	_ = seed.Store.Save(&Token{AccessToken: "OLD", RefreshToken: "RT0", ExpiresAt: now.Unix() - 1, ClientID: "CID"})

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Go(func() {
			m := newManager(t, dir, f) // separate instance, like a separate process
			if _, err := m.AccessToken(context.Background()); err != nil {
				errs <- err
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("a concurrent caller failed: %v", err)
	}
	if n := refreshes.Load(); n != 1 {
		t.Errorf("%d refreshes, want 1", n)
	}
	stored, err := seed.Store.Load("CID")
	if err != nil || stored.RefreshToken != current.Load().(string) {
		t.Errorf("stored refresh token %+v (%v) must match the server's current one", stored, err)
	}
}

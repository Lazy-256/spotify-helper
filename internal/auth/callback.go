package auth

import (
	"context"
	"crypto/subtle"
	"errors"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"github.com/Lazy-256/spotify-helper/internal/output"
)

// Listener receives the browser redirect of one login attempt.
type Listener struct {
	ln      net.Listener
	srv     *http.Server
	path    string
	state   string
	results chan callbackResult
}

type callbackResult struct {
	code string
	err  error
}

// Listen binds the host and port of redirectURI (loopback only) so a busy port
// is reported before the user is sent to the browser.
func Listen(redirectURI, state string) (*Listener, error) {
	u, err := url.Parse(redirectURI)
	if err != nil {
		return nil, output.Errorf(output.CodeNotConfigured, "bad redirect_uri: %v", err)
	}
	ln, err := net.Listen("tcp", u.Host)
	if err != nil {
		return nil, output.Errorf(output.CodeInternal,
			"cannot listen on %s for the login callback (port %s busy?): %v", u.Host, u.Port(), err)
	}
	l := &Listener{
		ln:      ln,
		path:    u.Path,
		state:   state,
		results: make(chan callbackResult, 1),
	}
	l.srv = &http.Server{
		Handler:           http.HandlerFunc(l.handle),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() { _ = l.srv.Serve(ln) }()
	return l, nil
}

// Wait blocks until a valid callback arrives or ctx ends.
func (l *Listener) Wait(ctx context.Context) (code string, err error) {
	select {
	case r := <-l.results:
		return r.code, r.err
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", output.Errorf(output.CodeNotAuthenticated, "login timed out waiting for the browser")
		}
		return "", output.Errorf(output.CodeNotAuthenticated, "login was cancelled")
	}
}

// Close stops listening; the page already sent to the browser is delivered.
func (l *Listener) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := l.srv.Shutdown(ctx); err != nil {
		_ = l.srv.Close()
	}
}

var safeErrorName = regexp.MustCompile(`^[a-z_]{1,40}$`)

func (l *Listener) handle(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != l.path {
		http.NotFound(w, r)
		return
	}
	q := r.URL.Query()
	if subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(l.state)) != 1 {
		// Not our login (or a forged request): ignore and keep waiting.
		page(w, http.StatusBadRequest, "Invalid request", "This login request is not recognised.")
		return
	}
	if e := q.Get("error"); e != "" {
		if !safeErrorName.MatchString(e) {
			e = "unknown_error"
		}
		page(w, http.StatusOK, "Not authorized", "Spotify did not grant access. You can close this tab.")
		l.finish(callbackResult{err: output.Errorf(output.CodeNotAuthenticated, "authorization was not granted (%s)", e)})
		return
	}
	code := q.Get("code")
	if code == "" {
		page(w, http.StatusBadRequest, "Invalid request", "The authorization code is missing.")
		return
	}
	page(w, http.StatusOK, "Logged in", "spotify-helper is authorized. You can close this tab.")
	l.finish(callbackResult{code: code})
}

// finish records the first outcome; later callbacks are ignored.
func (l *Listener) finish(r callbackResult) {
	select {
	case l.results <- r:
	default:
	}
}

func page(w http.ResponseWriter, status int, title, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write([]byte("<!doctype html><meta charset=utf-8><title>" + title + "</title>" +
		"<body style=\"font-family:sans-serif;margin:3em\"><h1>" + title + "</h1><p>" + body + "</p></body>"))
}

package cli

import (
	"context"
	"flag"
	"fmt"

	"github.com/Lazy-256/spotify-helper/internal/auth"
	"github.com/Lazy-256/spotify-helper/internal/output"
)

func init() {
	register(&command{
		name:    "login",
		usage:   "",
		summary: "authorize the helper in your browser (OAuth PKCE) and store the token",
		run:     runLogin,
	})
	register(&command{
		name:    "logout",
		usage:   "",
		summary: "delete the stored token",
		run:     runLogout,
	})
}

func runLogin(env *Env, args []string) error {
	pos, err := parseArgs(flag.NewFlagSet("login", flag.ContinueOnError), args)
	if err != nil {
		return err
	}
	if err := needArgs("login", "", pos, 0, 0); err != nil {
		return err
	}
	p, cfg, err := env.loadConfig()
	if err != nil {
		return err
	}

	verifier, err := auth.NewVerifier()
	if err != nil {
		return output.Wrap(err)
	}
	state, err := auth.NewState()
	if err != nil {
		return output.Wrap(err)
	}

	// Bind the port first so a busy port is reported before the browser opens.
	l, err := auth.Listen(cfg.RedirectURI, state)
	if err != nil {
		return err
	}
	defer l.Close()

	// The authorization URL necessarily carries the client ID, which is why
	// it is the one thing allowed on stderr that holds configuration.
	url := auth.AuthorizeURL(env.AccountsBase, cfg.ClientID, cfg.RedirectURI, auth.Challenge(verifier), state)
	fmt.Fprintf(env.Stderr, "Open this URL in your browser to authorize spotify-helper:\n%s\n", url)
	_ = env.OpenBrowser(url) // best effort; the URL above works by hand

	ctx, cancel := context.WithTimeout(context.Background(), env.LoginTimeout)
	defer cancel()
	code, err := l.Wait(ctx)
	if err != nil {
		return err
	}

	tok, err := env.endpoint(p, cfg).Exchange(ctx, code, verifier, cfg.RedirectURI)
	if err != nil {
		return err
	}
	if err := env.store(p).Save(tok); err != nil {
		return output.Errorf(output.CodeInternal, "cannot store the token: %v", err)
	}

	scopes := tok.Scopes
	if scopes == nil {
		scopes = []string{}
	}
	return output.WriteJSON(env.Stdout, struct {
		OK     bool     `json:"ok"`
		Scopes []string `json:"scopes"`
	}{true, scopes})
}

func runLogout(env *Env, args []string) error {
	pos, err := parseArgs(flag.NewFlagSet("logout", flag.ContinueOnError), args)
	if err != nil {
		return err
	}
	if err := needArgs("logout", "", pos, 0, 0); err != nil {
		return err
	}
	p, err := env.paths()
	if err != nil {
		return err
	}
	if err := env.store(p).Delete(); err != nil {
		return output.Errorf(output.CodeInternal, "cannot delete the token: %v", err)
	}
	return output.WriteOK(env.Stdout)
}

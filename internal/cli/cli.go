// Package cli wires subcommands to the packages that do the work. Everything
// the commands touch from the outside (streams, environment, clock, HTTP
// endpoints, browser) lives in Env so tests can substitute it.
package cli

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/Lazy-256/spotify-helper/internal/output"
)

// Version is set at build time with -ldflags "-X .../internal/cli.Version=…".
var Version = "dev"

const (
	defaultAPIBase      = "https://api.spotify.com/v1"
	defaultAccountsBase = "https://accounts.spotify.com"
)

// Env is everything a command needs from the outside world.
type Env struct {
	Stdout, Stderr io.Writer
	Getenv         func(string) string
	Now            func() time.Time

	// HTTP is used for api.spotify.com and accounts.spotify.com. It carries
	// credentials, so it must never be used for image downloads.
	HTTP         *http.Client
	APIBase      string
	AccountsBase string

	// CoverHTTP downloads cover images. It is deliberately a different client
	// from HTTP so credentials can never reach the image CDN.
	CoverHTTP *http.Client

	// OpenBrowser is best-effort; its error is ignored by callers.
	OpenBrowser func(url string) error

	// LoginTimeout bounds how long `login` waits for the browser.
	LoginTimeout time.Duration
}

// DefaultEnv returns the production environment.
func DefaultEnv() *Env {
	return &Env{
		Stdout:       os.Stdout,
		Stderr:       os.Stderr,
		Getenv:       os.Getenv,
		Now:          time.Now,
		HTTP:         &http.Client{Timeout: 10 * time.Second},
		CoverHTTP:    &http.Client{Timeout: 10 * time.Second},
		APIBase:      defaultAPIBase,
		AccountsBase: defaultAccountsBase,
		LoginTimeout: 5 * time.Minute,
		OpenBrowser: func(url string) error {
			return exec.Command("xdg-open", url).Start()
		},
	}
}

// command is one subcommand. usage is the argument synopsis shown by --help.
type command struct {
	name    string
	usage   string
	summary string
	run     func(env *Env, args []string) error
}

var registry = map[string]*command{}

func register(c *command) {
	if _, dup := registry[c.name]; dup {
		panic("cli: duplicate command " + c.name)
	}
	registry[c.name] = c
}

// Run executes one invocation and returns the process exit code. It is the
// only place that prints errors.
func Run(args []string, env *Env) int {
	err := run(args, env)
	if err == nil {
		return 0
	}
	oe := output.Wrap(err)
	output.WriteError(env.Stderr, oe)
	return oe.Code.ExitCode()
}

func isHelp(a string) bool { return a == "-h" || a == "--help" || a == "help" }

func run(args []string, env *Env) error {
	if len(args) == 0 {
		return output.Usage("missing command; run `spotify-helper --help`")
	}
	if isHelp(args[0]) {
		_, err := io.WriteString(env.Stdout, helpText())
		return err
	}
	cmd, ok := registry[args[0]]
	if !ok {
		return output.Usage("unknown command %q; run `spotify-helper --help`", args[0])
	}
	for _, a := range args[1:] {
		if a == "--" {
			break
		}
		if a == "-h" || a == "--help" {
			_, err := io.WriteString(env.Stdout, commandHelp(cmd))
			return err
		}
	}
	return cmd.run(env, args[1:])
}

func helpText() string {
	names := make([]string, 0, len(registry))
	for n := range registry {
		names = append(names, n)
	}
	sort.Strings(names)
	var b strings.Builder
	b.WriteString("spotify-helper — control Spotify through the Web API\n\n")
	b.WriteString("Usage: spotify-helper <command> [arguments]\n\nCommands:\n")
	for _, n := range names {
		c := registry[n]
		fmt.Fprintf(&b, "  %-12s %s\n", c.name, c.summary)
	}
	b.WriteString("\nRun `spotify-helper <command> --help` for a command's arguments.\n")
	b.WriteString("Output is JSON on stdout; errors are JSON on stderr with a non-zero exit code.\n")
	return b.String()
}

func commandHelp(c *command) string {
	return fmt.Sprintf("Usage: spotify-helper %s %s\n\n%s\n", c.name, c.usage, c.summary)
}

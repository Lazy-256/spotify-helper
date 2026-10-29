package cli

import (
	"context"
	"flag"
	"time"

	"github.com/Lazy-256/spotify-helper/internal/api"
	"github.com/Lazy-256/spotify-helper/internal/auth"
	"github.com/Lazy-256/spotify-helper/internal/config"
)

const (
	// opTimeout bounds one whole command, including a token refresh.
	opTimeout = 30 * time.Second
	// bulkTimeout bounds the commands that send up to a hundred sequential
	// requests (queue-add-all, queue-clear): 100 `next` take about 45 s. Every
	// request still has its own 10 s timeout.
	bulkTimeout = 120 * time.Second
)

func opContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), opTimeout)
}

// apiClient builds the Web API client. It is called only after the arguments
// have been validated, so usage errors never depend on configuration.
func (e *Env) apiClient() (*api.Client, config.Paths, error) {
	p, cfg, err := e.loadConfig()
	if err != nil {
		return nil, p, err
	}
	mgr := &auth.Manager{
		Store:    e.store(p),
		Endpoint: e.endpoint(p, cfg),
		LockPath: p.TokenLockFile(),
	}
	return &api.Client{
		HTTP:   e.HTTP,
		Base:   e.APIBase,
		Tokens: mgr,
		Gate:   e.gate(p),
		Now:    e.Now,

		SettleDelay: e.SettleDelay,
	}, p, nil
}

// commonFlags are the flags shared by several commands.
type commonFlags struct {
	device string
	covers bool
}

type flagSet struct {
	device, covers bool
	extra          func(fs *flag.FlagSet)
}

// parseCommon parses flags and positionals and checks the positional count.
func parseCommon(name, usage string, args []string, min, max int, opts flagSet) ([]string, commonFlags, error) {
	var cf commonFlags
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	if opts.device {
		fs.StringVar(&cf.device, "device", "", "target device id")
	}
	if opts.covers {
		fs.BoolVar(&cf.covers, "covers", false, "add local cover_path to results")
	}
	if opts.extra != nil {
		opts.extra(fs)
	}
	pos, err := parseArgs(fs, args)
	if err != nil {
		return nil, cf, err
	}
	return pos, cf, needArgs(name, usage, pos, min, max)
}

package cli

import (
	"context"
	"flag"

	"github.com/Lazy-256/spotify-helper/internal/api"
	"github.com/Lazy-256/spotify-helper/internal/output"
)

func init() {
	register(&command{name: "recent", usage: "[--limit 1-50] [--before MS | --after MS] [--covers]", summary: "show your recently played tracks", run: runRecent})
	register(&command{name: "top", usage: "<tracks|artists> [--range short|medium|long] [--limit 1-50] [--offset N] [--covers]", summary: "show your top tracks or artists", run: runTop})
}

// defaultHistoryLimit is the page size of recent and top when --limit is absent.
const defaultHistoryLimit = 20

func checkHistoryLimit(limit int) error {
	if limit < 1 || limit > api.MaxHistoryLimit {
		return output.Usage("--limit must be between 1 and %d, got %d", api.MaxHistoryLimit, limit)
	}
	return nil
}

func runRecent(env *Env, args []string) error {
	var (
		limit         int
		before, after int64
		fs            *flag.FlagSet
	)
	_, f, err := parseCommon("recent", "[--limit 1-50] [--before MS | --after MS] [--covers]", args, 0, 0, flagSet{covers: true, extra: func(s *flag.FlagSet) {
		fs = s
		s.IntVar(&limit, "limit", defaultHistoryLimit, "page size, 1-50")
		s.Int64Var(&before, "before", 0, "only plays before this Unix time in milliseconds")
		s.Int64Var(&after, "after", 0, "only plays after this Unix time in milliseconds")
	}})
	if err != nil {
		return err
	}
	if err := checkHistoryLimit(limit); err != nil {
		return err
	}
	// The flags are told apart from their zero values by whether they were given.
	var beforeP, afterP *int64
	fs.Visit(func(fl *flag.Flag) {
		switch fl.Name {
		case "before":
			beforeP = &before
		case "after":
			afterP = &after
		}
	})
	if beforeP != nil && afterP != nil {
		return output.Usage("--before and --after cannot be used together")
	}
	if (beforeP != nil && before < 0) || (afterP != nil && after < 0) {
		return output.Usage("--before and --after take a Unix time in milliseconds, not a negative number")
	}
	return withClient(env, func(ctx context.Context, c *api.Client) error {
		page, err := c.Recent(ctx, limit, beforeP, afterP)
		if err != nil {
			return err
		}
		if err := env.attachCovers(ctx, f.covers, page.Covers()); err != nil {
			return err
		}
		return output.WriteJSON(env.Stdout, page)
	})
}

func runTop(env *Env, args []string) error {
	var (
		limit, offset int
		rng           string
	)
	pos, f, err := parseCommon("top", "<tracks|artists> [--range short|medium|long] [--limit 1-50] [--offset N] [--covers]", args, 1, 1, flagSet{covers: true, extra: func(s *flag.FlagSet) {
		s.StringVar(&rng, "range", "medium", "short (4 weeks), medium (6 months) or long (a year)")
		s.IntVar(&limit, "limit", defaultHistoryLimit, "page size, 1-50")
		s.IntVar(&offset, "offset", 0, "index of the first entry")
	}})
	if err != nil {
		return err
	}
	kind := pos[0]
	if kind != "tracks" && kind != "artists" {
		return output.Usage("top needs tracks or artists, got %q", kind)
	}
	if _, ok := api.TopRanges[rng]; !ok {
		return output.Usage("--range must be short, medium or long, got %q", rng)
	}
	if err := checkHistoryLimit(limit); err != nil {
		return err
	}
	if offset < 0 {
		return output.Usage("--offset must not be negative, got %d", offset)
	}
	return withClient(env, func(ctx context.Context, c *api.Client) error {
		res, err := c.Top(ctx, kind, rng, limit, offset)
		if err != nil {
			return err
		}
		if err := env.attachCovers(ctx, f.covers, res.Covers()); err != nil {
			return err
		}
		return output.WriteJSON(env.Stdout, res)
	})
}

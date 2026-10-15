package cli

import (
	"context"
	"flag"
	"slices"
	"strings"

	"github.com/Lazy-256/spotify-helper/internal/api"
	"github.com/Lazy-256/spotify-helper/internal/output"
)

func init() {
	register(&command{
		name:    "search",
		usage:   "<query> [--type track,album,artist,playlist] [--limit 1-10] [--covers]",
		summary: "search tracks, albums, artists and playlists",
		run:     runSearch,
	})
}

func runSearch(env *Env, args []string) error {
	var typeArg string
	var limit int
	pos, f, err := parseCommon("search", "<query> [--type …] [--limit N] [--covers]", args, 1, -1,
		flagSet{covers: true, extra: func(fs *flag.FlagSet) {
			fs.StringVar(&typeArg, "type", "", "comma-separated subset of track,album,artist,playlist")
			fs.IntVar(&limit, "limit", api.MaxSearchLimit, "results per type, 1-10")
		}})
	if err != nil {
		return err
	}
	query := strings.Join(strings.Fields(strings.Join(pos, " ")), " ")
	if query == "" {
		return output.Usage("usage: spotify-helper search <query> [--type …] [--limit N]")
	}
	types, err := parseSearchTypes(typeArg)
	if err != nil {
		return err
	}
	if limit < 1 {
		return output.Usage("--limit must be between 1 and %d, got %d", api.MaxSearchLimit, limit)
	}
	limit = min(limit, api.MaxSearchLimit)

	return withClient(env, func(ctx context.Context, c *api.Client) error {
		res, err := c.Search(ctx, query, types, limit)
		if err != nil {
			return err
		}
		if err := env.attachCovers(ctx, f.covers, res.Covers()); err != nil {
			return err
		}
		return output.WriteJSON(env.Stdout, res)
	})
}

// parseSearchTypes returns the requested types in canonical order; empty
// input means all of them.
func parseSearchTypes(s string) ([]string, error) {
	if strings.TrimSpace(s) == "" {
		return slices.Clone(api.SearchTypes), nil
	}
	want := map[string]bool{}
	for _, t := range strings.Split(s, ",") {
		t = strings.ToLower(strings.TrimSpace(t))
		if !slices.Contains(api.SearchTypes, t) {
			return nil, output.Usage("unknown search type %q; use track, album, artist or playlist", t)
		}
		want[t] = true
	}
	var out []string
	for _, t := range api.SearchTypes {
		if want[t] {
			out = append(out, t)
		}
	}
	return out, nil
}

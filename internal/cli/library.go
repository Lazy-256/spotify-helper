package cli

import (
	"context"
	"flag"

	"github.com/Lazy-256/spotify-helper/internal/api"
	"github.com/Lazy-256/spotify-helper/internal/output"
)

func init() {
	register(&command{name: "playlists", usage: "[--limit 1-50] [--offset N] [--covers]", summary: "list your playlists", run: runPlaylists})
	register(&command{name: "albums", usage: "[--limit 1-50] [--offset N] [--covers]", summary: "list your saved albums", run: runAlbums})
}

// pageArgs parses the paging flags shared by the library commands.
func pageArgs(name string, args []string) (limit, offset int, f commonFlags, err error) {
	limit = api.MaxLibraryLimit
	usage := "[--limit 1-50] [--offset N] [--covers]"
	_, f, err = parseCommon(name, usage, args, 0, 0, flagSet{covers: true, extra: func(fs *flag.FlagSet) {
		fs.IntVar(&limit, "limit", api.MaxLibraryLimit, "page size, 1-50")
		fs.IntVar(&offset, "offset", 0, "index of the first entry")
	}})
	if err != nil {
		return
	}
	if limit < 1 || limit > api.MaxLibraryLimit {
		err = output.Usage("--limit must be between 1 and %d, got %d", api.MaxLibraryLimit, limit)
	} else if offset < 0 {
		err = output.Usage("--offset must not be negative, got %d", offset)
	}
	return
}

func runPlaylists(env *Env, args []string) error {
	limit, offset, f, err := pageArgs("playlists", args)
	if err != nil {
		return err
	}
	return withClient(env, func(ctx context.Context, c *api.Client) error {
		page, err := c.Playlists(ctx, limit, offset)
		if err != nil {
			return err
		}
		if err := env.attachCovers(ctx, f.covers, page.Covers()); err != nil {
			return err
		}
		return output.WriteJSON(env.Stdout, page)
	})
}

func runAlbums(env *Env, args []string) error {
	limit, offset, f, err := pageArgs("albums", args)
	if err != nil {
		return err
	}
	return withClient(env, func(ctx context.Context, c *api.Client) error {
		page, err := c.Albums(ctx, limit, offset)
		if err != nil {
			return err
		}
		if err := env.attachCovers(ctx, f.covers, page.Covers()); err != nil {
			return err
		}
		return output.WriteJSON(env.Stdout, page)
	})
}

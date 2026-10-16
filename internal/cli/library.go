package cli

import (
	"context"
	"flag"

	"github.com/Lazy-256/spotify-helper/internal/api"
	"github.com/Lazy-256/spotify-helper/internal/output"
	"github.com/Lazy-256/spotify-helper/internal/spotifyuri"
)

func init() {
	register(&command{name: "playlists", usage: "[--limit 1-50] [--offset N] [--covers]", summary: "list your playlists", run: runPlaylists})
	register(&command{name: "save", usage: "<track-or-episode uri|link>", summary: "add a track or episode to your library", run: runSave})
	register(&command{name: "unsave", usage: "<track-or-episode uri|link>", summary: "remove a track or episode from your library", run: runUnsave})
	register(&command{name: "saved", usage: "<track-or-episode uri|link>", summary: "tell whether a track or episode is in your library", run: runSaved})
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

// libraryItem parses the single URI argument of save, unsave and saved.
func libraryItem(name string, args []string) (spotifyuri.URI, error) {
	pos, _, err := parseCommon(name, "<track-or-episode uri|link>", args, 1, 1, flagSet{})
	if err != nil {
		return spotifyuri.URI{}, err
	}
	return spotifyuri.ParseAllowed(pos[0], spotifyuri.Track, spotifyuri.Episode)
}

func runSave(env *Env, args []string) error {
	return writeLibraryItem(env, "save", args, (*api.Client).SaveItem)
}

func runUnsave(env *Env, args []string) error {
	return writeLibraryItem(env, "unsave", args, (*api.Client).RemoveItem)
}

func writeLibraryItem(env *Env, name string, args []string, call func(*api.Client, context.Context, spotifyuri.URI) error) error {
	u, err := libraryItem(name, args)
	if err != nil {
		return err
	}
	return withClient(env, func(ctx context.Context, c *api.Client) error {
		if err := call(c, ctx, u); err != nil {
			return err
		}
		return output.WriteJSON(env.Stdout, struct {
			OK  bool   `json:"ok"`
			URI string `json:"uri"`
		}{true, u.String()})
	})
}

func runSaved(env *Env, args []string) error {
	u, err := libraryItem("saved", args)
	if err != nil {
		return err
	}
	return withClient(env, func(ctx context.Context, c *api.Client) error {
		saved, err := c.ItemSaved(ctx, u)
		if err != nil {
			return err
		}
		return output.WriteJSON(env.Stdout, struct {
			URI   string `json:"uri"`
			Saved bool   `json:"saved"`
		}{u.String(), saved})
	})
}

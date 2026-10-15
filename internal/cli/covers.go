package cli

import (
	"context"

	"github.com/Lazy-256/spotify-helper/internal/covers"
	"github.com/Lazy-256/spotify-helper/internal/output"
)

func init() {
	register(&command{
		name:    "cover",
		usage:   "<https-url>",
		summary: "download an image into the cover cache and print its local path",
		run:     runCover,
	})
}

func (e *Env) coverCache() (*covers.Cache, error) {
	p, err := e.paths()
	if err != nil {
		return nil, err
	}
	return &covers.Cache{Dir: p.CoversDir(), HTTP: e.CoverHTTP, Now: e.Now}, nil
}

// attachCovers fills cover_path for the given covers when --covers was given.
// It never fails because of a bad or slow image: those become null.
func (e *Env) attachCovers(ctx context.Context, requested bool, cvs []*output.Cover) error {
	if !requested {
		return nil
	}
	cache, err := e.coverCache()
	if err != nil {
		return err
	}
	cache.Apply(ctx, cvs)
	return nil
}

func runCover(env *Env, args []string) error {
	pos, _, err := parseCommon("cover", "<https-url>", args, 1, 1, flagSet{})
	if err != nil {
		return err
	}
	cache, err := env.coverCache()
	if err != nil {
		return err
	}
	ctx, cancel := opContext()
	defer cancel()
	path, cached, err := cache.Fetch(ctx, pos[0])
	if err != nil {
		return err
	}
	if !cached {
		cache.MaybeCleanup()
	}
	return output.WriteJSON(env.Stdout, struct {
		OK     bool   `json:"ok"`
		URL    string `json:"url"`
		Path   string `json:"path"`
		Cached bool   `json:"cached"`
	}{true, pos[0], path, cached})
}

package cli

import (
	"context"

	"github.com/Lazy-256/spotify-helper/internal/api"
	"github.com/Lazy-256/spotify-helper/internal/output"
	"github.com/Lazy-256/spotify-helper/internal/spotifyuri"
)

func init() {
	register(&command{name: "album", usage: "<uri|link> [--limit 1-50] [--offset N] [--covers]", summary: "show an album and its tracks", run: runAlbum})
	register(&command{name: "playlist", usage: "<uri|link> [--limit 1-50] [--offset N] [--covers]", summary: "show a playlist and, when Spotify allows it, its tracks", run: runPlaylist})
	register(&command{name: "artist", usage: "<uri|link> [--limit 1-10] [--offset N] [--covers]", summary: "show an artist and their albums and singles", run: runArtist})
}

// catalogArgs parses `<uri|link> [--limit] [--offset] [--covers]` and checks
// that the URI is of the given type, before anything is sent to Spotify.
func catalogArgs(name string, typ spotifyuri.Type, args []string) (u spotifyuri.URI, limit, offset int, f commonFlags, err error) {
	pos, limit, offset, f, err := pagedArgs(name, "<uri|link> [--limit 1-50] [--offset N] [--covers]", args, 1, 1)
	if err != nil {
		return
	}
	u, err = spotifyuri.ParseAllowed(pos[0], typ)
	return
}

func runAlbum(env *Env, args []string) error {
	u, limit, offset, f, err := catalogArgs("album", spotifyuri.Album, args)
	if err != nil {
		return err
	}
	return withClient(env, func(ctx context.Context, c *api.Client) error {
		res, err := c.Album(ctx, u, limit, offset)
		if err != nil {
			return err
		}
		if err := env.attachCovers(ctx, f.covers, res.Covers()); err != nil {
			return err
		}
		return output.WriteJSON(env.Stdout, res)
	})
}

func runPlaylist(env *Env, args []string) error {
	u, limit, offset, f, err := catalogArgs("playlist", spotifyuri.Playlist, args)
	if err != nil {
		return err
	}
	return withClient(env, func(ctx context.Context, c *api.Client) error {
		res, err := c.Playlist(ctx, u, limit, offset)
		if err != nil {
			return err
		}
		if err := env.attachCovers(ctx, f.covers, res.Covers()); err != nil {
			return err
		}
		return output.WriteJSON(env.Stdout, res)
	})
}

func runArtist(env *Env, args []string) error {
	u, limit, offset, f, err := catalogArgs("artist", spotifyuri.Artist, args)
	if err != nil {
		return err
	}
	// Spotify answers HTTP 400 "Invalid limit" to an artist's albums for more than
	// 10 entries in Development Mode, so a larger page (including the default) is
	// reduced, as search does.
	limit = min(limit, api.MaxArtistAlbumsLimit)
	return withClient(env, func(ctx context.Context, c *api.Client) error {
		res, err := c.Artist(ctx, u, limit, offset)
		if err != nil {
			return err
		}
		if err := env.attachCovers(ctx, f.covers, res.Covers()); err != nil {
			return err
		}
		return output.WriteJSON(env.Stdout, res)
	})
}

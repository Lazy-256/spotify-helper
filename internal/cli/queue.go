package cli

import (
	"context"
	"flag"

	"github.com/Lazy-256/spotify-helper/internal/api"
	"github.com/Lazy-256/spotify-helper/internal/output"
	"github.com/Lazy-256/spotify-helper/internal/spotifyuri"
)

const queueAddAllUsage = "<album|playlist uri|link> [--offset N] [--device ID]"

func init() {
	register(&command{name: "queue", usage: "[--covers]", summary: "show what is playing now and what comes next", run: runQueue})
	register(&command{name: "queue-add", usage: "<track-or-episode uri|link> [--device ID]", summary: "append a track or episode to the queue", run: runQueueAdd})
	register(&command{name: "queue-add-all", usage: queueAddAllUsage, summary: "append an album or playlist (up to 100 items) to the queue", run: runQueueAddAll})
	register(&command{name: "queue-clear", usage: "", summary: "drop the playback context and play through the queue (up to 100 entries), then pause", run: runQueueClear})
}

func runQueue(env *Env, args []string) error {
	_, f, err := parseCommon("queue", "[--covers]", args, 0, 0, flagSet{covers: true})
	if err != nil {
		return err
	}
	return withClient(env, func(ctx context.Context, c *api.Client) error {
		q, err := c.Queue(ctx)
		if err != nil {
			return err
		}
		if err := env.attachCovers(ctx, f.covers, q.Covers()); err != nil {
			return err
		}
		return output.WriteJSON(env.Stdout, q)
	})
}

func runQueueAdd(env *Env, args []string) error {
	pos, f, err := parseCommon("queue-add", "<track-or-episode uri|link> [--device ID]", args, 1, 1, flagSet{device: true})
	if err != nil {
		return err
	}
	u, err := spotifyuri.ParseAllowed(pos[0], spotifyuri.Track, spotifyuri.Episode)
	if err != nil {
		return err
	}
	return withClient(env, func(ctx context.Context, c *api.Client) error {
		if err := c.QueueAdd(ctx, u, f.device); err != nil {
			return err
		}
		return output.WriteJSON(env.Stdout, struct {
			OK  bool   `json:"ok"`
			URI string `json:"uri"`
		}{true, u.String()})
	})
}

func runQueueAddAll(env *Env, args []string) error {
	var offset int
	pos, f, err := parseCommon("queue-add-all", queueAddAllUsage, args, 1, 1, flagSet{device: true, extra: func(fs *flag.FlagSet) {
		fs.IntVar(&offset, "offset", 0, "position of the first item to queue")
	}})
	if err != nil {
		return err
	}
	if offset < 0 {
		return output.Usage("--offset must not be negative, got %d", offset)
	}
	u, err := spotifyuri.ParseAllowed(pos[0], spotifyuri.Album, spotifyuri.Playlist)
	if err != nil {
		return err
	}
	return withBulkClient(env, func(ctx context.Context, c *api.Client) error {
		res, err := c.QueueAddAll(ctx, u, offset, f.device)
		if err != nil {
			return err
		}
		return output.WriteJSON(env.Stdout, res)
	})
}

func runQueueClear(env *Env, args []string) error {
	_, _, err := parseCommon("queue-clear", "", args, 0, 0, flagSet{})
	if err != nil {
		return err
	}
	return withBulkClient(env, func(ctx context.Context, c *api.Client) error {
		res, err := c.QueueClear(ctx)
		if err != nil {
			return err
		}
		return output.WriteJSON(env.Stdout, res)
	})
}

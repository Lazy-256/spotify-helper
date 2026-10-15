package cli

import (
	"context"

	"github.com/Lazy-256/spotify-helper/internal/api"
	"github.com/Lazy-256/spotify-helper/internal/output"
	"github.com/Lazy-256/spotify-helper/internal/spotifyuri"
)

func init() {
	register(&command{name: "queue", usage: "[--covers]", summary: "show what is playing now and what comes next", run: runQueue})
	register(&command{name: "queue-add", usage: "<track-or-episode uri|link> [--device ID]", summary: "append a track or episode to the queue", run: runQueueAdd})
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

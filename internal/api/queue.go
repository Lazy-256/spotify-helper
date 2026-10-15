package api

import (
	"context"
	"net/http"
	"net/url"

	"github.com/Lazy-256/spotify-helper/internal/output"
	"github.com/Lazy-256/spotify-helper/internal/spotifyuri"
)

// Queue returns the item playing now and the upcoming items exactly as
// Spotify reports them. Entries that are not tracks or episodes (ads) are
// dropped.
func (c *Client) Queue(ctx context.Context) (*output.QueueResult, error) {
	var w struct {
		CurrentlyPlaying *wireItem   `json:"currently_playing"`
		Queue            []*wireItem `json:"queue"`
	}
	if _, err := c.do(ctx, http.MethodGet, "/me/player/queue", nil, nil, &w); err != nil {
		return nil, err
	}
	res := &output.QueueResult{NowPlaying: mapItem(w.CurrentlyPlaying), Queue: []output.Item{}}
	for _, it := range w.Queue {
		if m := mapItem(it); m != nil {
			res.Queue = append(res.Queue, *m)
		}
	}
	return res, nil
}

// QueueAdd appends a track or episode to the queue of the active device, or
// of device when given.
func (c *Client) QueueAdd(ctx context.Context, u spotifyuri.URI, device string) error {
	q := deviceQuery(device, url.Values{"uri": {u.String()}})
	_, err := c.do(ctx, http.MethodPost, "/me/player/queue", q, nil, nil)
	return err
}

// Transfer moves playback to the device with the given id. With play false the
// current playing/paused state is kept.
func (c *Client) Transfer(ctx context.Context, deviceID string, play bool) error {
	body := map[string]any{"device_ids": []string{deviceID}}
	if play {
		body["play"] = true
	}
	_, err := c.do(ctx, http.MethodPut, "/me/player", nil, body, nil)
	return err
}

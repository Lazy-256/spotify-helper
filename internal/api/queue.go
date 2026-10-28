package api

import (
	"context"
	"errors"
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

const (
	// MaxQueueAll caps the items one queue-add-all run adds. It protects the
	// queue (Spotify cannot remove items) and the rate budget from a mistake.
	MaxQueueAll = 100
	// queueAllPage is the page size used to read the container.
	queueAllPage = 50
)

// QueueAddAll appends the tracks and episodes of the album or playlist u to
// the queue, starting at position offset, one request per item and strictly
// in container order. It adds at most MaxQueueAll items and stops at the first
// failure. A failure after some items were queued carries their number (and
// the container's total, when known) in the error, because they stay queued.
func (c *Client) QueueAddAll(ctx context.Context, u spotifyuri.URI, offset int, device string) (*output.QueueAddAllResult, error) {
	res := &output.QueueAddAllResult{OK: true, URI: u.String(), Offset: offset}
	totalKnown := false
	fail := func(err error) error {
		oe, ok := errors.AsType[*output.Error](err)
		if !ok || res.Added == 0 {
			return err
		}
		cp := *oe
		cp.Added = res.Added
		if totalKnown {
			cp.Total = res.Total
		}
		return &cp
	}

	pos := offset
	for {
		items, total, err := c.containerPage(ctx, u, pos)
		if err != nil {
			return nil, fail(err)
		}
		res.Total, totalKnown = total, true
		pageLen := min(queueAllPage, total-pos)
		if pageLen <= 0 {
			return res, nil
		}

		for i, it := range items {
			iu, perr := spotifyuri.Parse(it.URI)
			if perr != nil {
				res.Skipped++
				continue
			}
			if err := c.QueueAdd(ctx, iu, device); err != nil {
				return nil, fail(err)
			}
			res.Added++
			if res.Added >= MaxQueueAll {
				// Entries of this page after the one just queued are unexamined; when
				// there are none, the whole page was examined.
				if i == len(items)-1 {
					res.Skipped += pageLen - len(items)
					res.Truncated = pos+pageLen < total
				} else {
					res.Truncated = true
				}
				return res, nil
			}
		}
		// Entries of the page that did not map to an item (null, local files,
		// removed) cannot be queued.
		res.Skipped += pageLen - len(items)
		pos += pageLen
		if pos >= total {
			return res, nil
		}
	}
}

// containerPage reads one page of the album or playlist u at pos and returns
// the queueable items in order and the container's total. A playlist whose
// items Spotify does not serve fails with tracks_unavailable.
func (c *Client) containerPage(ctx context.Context, u spotifyuri.URI, pos int) ([]output.Item, int, error) {
	switch u.Type {
	case spotifyuri.Album:
		a, err := c.Album(ctx, u, queueAllPage, pos)
		if err != nil {
			return nil, 0, err
		}
		return a.Tracks, a.Total, nil
	case spotifyuri.Playlist:
		p, err := c.Playlist(ctx, u, queueAllPage, pos)
		if err != nil {
			return nil, 0, err
		}
		if !p.TracksAvailable {
			return nil, 0, output.Errorf(output.CodeTracksUnavailable, "Spotify does not serve the items of this playlist to this app")
		}
		return p.Tracks, p.Total, nil
	}
	return nil, 0, output.Usage("queue-add-all takes an album or a playlist, not %s", u.Type)
}

package api

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

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

// MaxQueueClear caps the `next` requests one queue-clear run sends. It keeps a
// run inside the rate window that 100 requests passed on a clean window; a
// caller repeats the command for the rest.
const MaxQueueClear = 100

// QueueClear empties the queue as far as the Web API allows. The API cannot
// remove entries, so it plays through them: it starts the current item again as
// its own context (which drops the rest of the old context while the queued
// entries stay), then sends one `next` per entry the queue really holds, reading
// the queue between batches of at most 20, up to MaxQueueClear requests, and pauses
// at the end, because every `next` starts an entry. A failure after the context
// was dropped carries the number of `next` requests already sent in the error.
func (c *Client) QueueClear(ctx context.Context, device string) (*output.QueueClearResult, error) {
	st, err := c.State(ctx)
	if err != nil {
		return nil, err
	}
	var current spotifyuri.URI
	if st.Active && st.Item != nil {
		current, err = spotifyuri.Parse(st.Item.URI)
	}
	if !st.Active || st.Item == nil || err != nil {
		return nil, output.Errorf(output.CodeNoActiveDevice, "nothing is playing; start playback on a device or pass --device")
	}
	if err := c.PlayURI(ctx, current, device); err != nil {
		return nil, err
	}

	res := &output.QueueClearResult{OK: true}
	fail := func(err error, pause bool) error {
		oe, ok := errors.AsType[*output.Error](err)
		if !ok {
			return err
		}
		// A rate limit or a lost connection would fail the pause the same way,
		// and a failed pause is not tried again.
		if pause && oe.Code != output.CodeRateLimited && oe.Code != output.CodeNetworkError {
			_ = c.Pause(ctx, device)
		}
		if res.Skipped == 0 {
			return err
		}
		cp := *oe
		cp.Skipped = res.Skipped
		return &cp
	}

	sent := false // `next` requests since the last queue read
loop:
	for {
		// A device applies `next` late, so a read right after a batch can still show
		// entries that are already used up; wait until two reads agree.
		read := c.Queue
		if sent {
			read = c.settledQueue
		}
		q, err := read(ctx)
		if err != nil {
			return nil, fail(err, true)
		}
		real := withoutPadding(q)
		if len(real) == 0 {
			res.Empty = true
			break
		}
		if res.Skipped >= MaxQueueClear {
			break
		}
		for range min(len(real), MaxQueueClear-res.Skipped) {
			for attempt := 0; ; attempt++ {
				err := c.Next(ctx, device)
				if err == nil {
					break
				}
				if !isRefused(err) {
					return nil, fail(err, true)
				}
				// Spotify refuses `next` when there is nothing to skip to, and also for a
				// moment while the device changes track. It is the end of the queue only if
				// the queue really holds nothing more; otherwise try again a few times.
				q, qerr := c.settledQueue(ctx)
				if qerr == nil && len(withoutPadding(q)) == 0 {
					res.Empty = true
					break loop
				}
				if attempt >= nextRetries {
					return nil, fail(err, true)
				}
			}
			res.Skipped++
		}
		sent = true
	}
	// A refused pause means no item is current: there is nothing to pause.
	if err := c.Pause(ctx, device); err != nil && !isRefused(err) {
		return nil, fail(err, false)
	}
	return res, nil
}

const (
	queueSettleDelay = 250 * time.Millisecond
	queueSettleReads = 4
	// nextRetries is how many times a refused `next` is tried again while the
	// queue still holds entries.
	nextRetries = 3
)

// settledQueue reads the queue until two reads in a row agree (the item playing
// and the entries), at most queueSettleReads more times, and returns the last one.
func (c *Client) settledQueue(ctx context.Context) (*output.QueueResult, error) {
	delay := c.SettleDelay
	if delay == 0 {
		delay = queueSettleDelay
	}
	prev, err := c.Queue(ctx)
	if err != nil {
		return nil, err
	}
	for range queueSettleReads {
		select {
		case <-ctx.Done():
			return nil, output.Network(ctx.Err())
		case <-time.After(delay):
		}
		cur, err := c.Queue(ctx)
		if err != nil {
			return nil, err
		}
		if sameQueue(prev, cur) {
			return cur, nil
		}
		prev = cur
	}
	return prev, nil
}

func sameQueue(a, b *output.QueueResult) bool {
	if (a.NowPlaying == nil) != (b.NowPlaying == nil) || len(a.Queue) != len(b.Queue) {
		return false
	}
	if a.NowPlaying != nil && a.NowPlaying.URI != b.NowPlaying.URI {
		return false
	}
	for i := range a.Queue {
		if a.Queue[i].URI != b.Queue[i].URI {
			return false
		}
	}
	return true
}

// isRefused reports Spotify's HTTP 403 "Restriction violated", which it answers
// to `next` and `pause` when no item is current or nothing follows.
func isRefused(err error) bool {
	oe, ok := errors.AsType[*output.Error](err)
	return ok && oe.Code == output.CodeAPIError && strings.Contains(oe.Message, "HTTP 403")
}

// withoutPadding returns the queue entries that are really queued. Once the
// queued entries are used up and the context is a single item, Spotify goes on
// showing copies of the item playing now at the end of the queue (found live,
// 2026-10-29); skipping them would never end, so a trailing run of entries
// with the URI of the current item is dropped. A real entry that is the same
// item as the current one and stands last is taken for padding too. Without a
// current item nothing counts, because `next` would be refused.
func withoutPadding(q *output.QueueResult) []output.Item {
	items := q.Queue
	if q.NowPlaying == nil {
		return nil // no item is current: nothing can be skipped to
	}
	for len(items) > 0 && items[len(items)-1].URI == q.NowPlaying.URI {
		items = items[:len(items)-1]
	}
	return items
}

// containerPage reads one page of the album or playlist u at pos and returns
// the queueable items in order and the container's total. Tracks that Spotify
// marks as not playable for the user are left out. A playlist whose
// items Spotify does not serve fails with tracks_unavailable.
func (c *Client) containerPage(ctx context.Context, u spotifyuri.URI, pos int) ([]output.Item, int, error) {
	switch u.Type {
	case spotifyuri.Album:
		a, err := c.album(ctx, u, queueAllPage, pos, "from_token")
		if err != nil {
			return nil, 0, err
		}
		return a.Tracks, a.Total, nil
	case spotifyuri.Playlist:
		p, err := c.playlist(ctx, u, queueAllPage, pos, "from_token")
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

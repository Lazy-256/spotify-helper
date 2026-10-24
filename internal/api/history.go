package api

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"github.com/Lazy-256/spotify-helper/internal/auth"
	"github.com/Lazy-256/spotify-helper/internal/output"
)

// MaxHistoryLimit is the largest page Spotify serves for the listening history
// and for top items.
const MaxHistoryLimit = 50

// requireScope fails with scopes_missing, before any request, when the token
// source knows the token lacks scope.
func (c *Client) requireScope(scope string) error {
	if sc, ok := c.Tokens.(ScopeChecker); ok {
		return sc.RequireScope(scope)
	}
	return nil
}

// wirePlay is one entry of the recently played list.
type wirePlay struct {
	PlayedAt string `json:"played_at"`
	Context  *struct {
		Type string `json:"type"`
		URI  string `json:"uri"`
	} `json:"context"`
	Track *wireItem `json:"track"`
}

// cursor parses one of Spotify's cursors, which come as decimal strings of
// Unix milliseconds; anything else is no cursor.
func cursor(s string) *int64 {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 0 {
		return nil
	}
	return &n
}

// Recent returns one page of the user's recently played tracks, newest first.
// before and after are Unix milliseconds and exclude each other; both nil asks
// for the latest plays. The output's before cursor is null when Spotify has no
// older page.
func (c *Client) Recent(ctx context.Context, limit int, before, after *int64) (*output.RecentPage, error) {
	if err := c.requireScope(auth.ScopeRecentlyPlayed); err != nil {
		return nil, err
	}
	q := url.Values{"limit": {strconv.Itoa(limit)}}
	if before != nil {
		q.Set("before", strconv.FormatInt(*before, 10))
	}
	if after != nil {
		q.Set("after", strconv.FormatInt(*after, 10))
	}
	var w struct {
		Next    string `json:"next"`
		Cursors *struct {
			Before string `json:"before"`
			After  string `json:"after"`
		} `json:"cursors"`
		Items []*wirePlay `json:"items"`
	}
	if _, err := c.do(ctx, http.MethodGet, "/me/player/recently-played", q, nil, &w); err != nil {
		return nil, err
	}
	res := &output.RecentPage{Limit: limit, Items: []output.PlayedItem{}}
	if w.Cursors != nil {
		if w.Next != "" {
			res.Cursors.Before = cursor(w.Cursors.Before)
		}
		res.Cursors.After = cursor(w.Cursors.After)
	}
	for _, p := range w.Items {
		if p == nil || p.Track == nil {
			continue
		}
		if p.Track.Type == "" {
			p.Track.Type = "track"
		}
		it := mapItem(p.Track)
		if it == nil {
			continue
		}
		played := output.PlayedItem{PlayedAt: p.PlayedAt, Track: *it}
		if p.Context != nil && p.Context.URI != "" {
			played.Context = &output.PlaybackContext{Type: p.Context.Type, URI: p.Context.URI}
		}
		res.Items = append(res.Items, played)
	}
	return res, nil
}

// TopRanges maps the helper's range names to Spotify's time ranges.
var TopRanges = map[string]string{"short": "short_term", "medium": "medium_term", "long": "long_term"}

// Top returns one page of the user's top tracks (kind "tracks") or artists
// (kind "artists") for a range of "short", "medium" or "long".
func (c *Client) Top(ctx context.Context, kind, rng string, limit, offset int) (*output.TopResult, error) {
	if err := c.requireScope(auth.ScopeTopRead); err != nil {
		return nil, err
	}
	q := pageQuery(limit, offset)
	q.Set("time_range", TopRanges[rng])
	res := &output.TopResult{Type: kind, Range: rng, Offset: offset, Limit: limit}
	if kind == "artists" {
		var w struct {
			Total int                `json:"total"`
			Items []*wireArtistEntry `json:"items"`
		}
		if _, err := c.do(ctx, http.MethodGet, "/me/top/artists", q, nil, &w); err != nil {
			return nil, err
		}
		list := []output.ArtistEntry{}
		for _, a := range w.Items {
			if a != nil {
				list = append(list, mapArtistEntry(*a))
			}
		}
		res.Total, res.Artists = w.Total, &list
		return res, nil
	}
	var w struct {
		Total int         `json:"total"`
		Items []*wireItem `json:"items"`
	}
	if _, err := c.do(ctx, http.MethodGet, "/me/top/tracks", q, nil, &w); err != nil {
		return nil, err
	}
	list := []output.Item{}
	for _, t := range w.Items {
		if t == nil {
			continue
		}
		if t.Type == "" {
			t.Type = "track"
		}
		if it := mapItem(t); it != nil {
			list = append(list, *it)
		}
	}
	res.Total, res.Tracks = w.Total, &list
	return res, nil
}

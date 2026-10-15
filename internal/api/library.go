package api

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"github.com/Lazy-256/spotify-helper/internal/output"
)

// MaxLibraryLimit is the largest page Spotify serves for library listings.
const MaxLibraryLimit = 50

func pageQuery(limit, offset int) url.Values {
	return url.Values{"limit": {strconv.Itoa(limit)}, "offset": {strconv.Itoa(offset)}}
}

// Playlists returns one page of the user's playlists. limit and offset echo
// the applied request; Total is the number of playlists available.
func (c *Client) Playlists(ctx context.Context, limit, offset int) (*output.PlaylistPage, error) {
	var w struct {
		Total int             `json:"total"`
		Items []*wirePlaylist `json:"items"`
	}
	if _, err := c.do(ctx, http.MethodGet, "/me/playlists", pageQuery(limit, offset), nil, &w); err != nil {
		return nil, err
	}
	page := &output.PlaylistPage{Total: w.Total, Offset: offset, Limit: limit, Playlists: []output.PlaylistEntry{}}
	for _, p := range w.Items {
		if p != nil {
			page.Playlists = append(page.Playlists, mapPlaylist(*p))
		}
	}
	return page, nil
}

// Albums returns one page of the user's saved albums.
func (c *Client) Albums(ctx context.Context, limit, offset int) (*output.AlbumPage, error) {
	var w struct {
		Total int `json:"total"`
		Items []*struct {
			Album *wireAlbum `json:"album"`
		} `json:"items"`
	}
	if _, err := c.do(ctx, http.MethodGet, "/me/albums", pageQuery(limit, offset), nil, &w); err != nil {
		return nil, err
	}
	page := &output.AlbumPage{Total: w.Total, Offset: offset, Limit: limit, Albums: []output.AlbumEntry{}}
	for _, it := range w.Items {
		if it != nil && it.Album != nil {
			page.Albums = append(page.Albums, mapAlbum(*it.Album))
		}
	}
	return page, nil
}

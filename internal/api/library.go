package api

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"github.com/Lazy-256/spotify-helper/internal/auth"
	"github.com/Lazy-256/spotify-helper/internal/output"
	"github.com/Lazy-256/spotify-helper/internal/spotifyuri"
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

// ScopeChecker is implemented by token sources that know which scopes the
// stored token was granted (see auth.Manager).
type ScopeChecker interface {
	RequireScope(scope string) error
}

func libraryQuery(u spotifyuri.URI) url.Values { return url.Values{"uris": {u.String()}} }

// SaveItem adds a track or episode to the user's library. Spotify answers 200
// whether or not the item was already there.
func (c *Client) SaveItem(ctx context.Context, u spotifyuri.URI) error {
	return c.writeLibrary(ctx, http.MethodPut, u)
}

// RemoveItem removes a track or episode from the user's library. Spotify
// answers 200 whether or not the item was there.
func (c *Client) RemoveItem(ctx context.Context, u spotifyuri.URI) error {
	return c.writeLibrary(ctx, http.MethodDelete, u)
}

// writeLibrary checks the token's scopes first (when the token source can),
// so a token from before user-library-modify fails without a request.
func (c *Client) writeLibrary(ctx context.Context, method string, u spotifyuri.URI) error {
	if sc, ok := c.Tokens.(ScopeChecker); ok {
		if err := sc.RequireScope(auth.ScopeLibraryModify); err != nil {
			return err
		}
	}
	_, err := c.do(ctx, method, "/me/library", libraryQuery(u), nil, nil)
	return err
}

// ItemSaved reports whether a track or episode is in the user's library.
// Spotify answers with a bare array holding one boolean per URI asked about.
func (c *Client) ItemSaved(ctx context.Context, u spotifyuri.URI) (bool, error) {
	var flags []bool
	if _, err := c.do(ctx, http.MethodGet, "/me/library/contains", libraryQuery(u), nil, &flags); err != nil {
		return false, err
	}
	if len(flags) != 1 {
		return false, output.Errorf(output.CodeAPIError, "unexpected answer from Spotify: expected one boolean, got %d values", len(flags))
	}
	return flags[0], nil
}

package api

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/Lazy-256/spotify-helper/internal/output"
)

// SearchTypes are the catalog types `search` knows, in output order.
var SearchTypes = []string{"track", "album", "artist", "playlist"}

// MaxSearchLimit is Spotify's cap for apps in Development Mode.
const MaxSearchLimit = 10

type wireSearch struct {
	Tracks    *struct{ Items []*wireItem }        `json:"tracks"`
	Albums    *struct{ Items []*wireAlbum }       `json:"albums"`
	Artists   *struct{ Items []*wireArtistEntry } `json:"artists"`
	Playlists *struct{ Items []*wirePlaylist }    `json:"playlists"`
}

// Search looks up query for the given types (a subset of SearchTypes) and
// returns at most limit entries per type. Entries Spotify returns as null are
// dropped.
func (c *Client) Search(ctx context.Context, query string, types []string, limit int) (*output.SearchResult, error) {
	limit = min(max(limit, 1), MaxSearchLimit)
	q := url.Values{
		"q":     {query},
		"type":  {strings.Join(types, ",")},
		"limit": {strconv.Itoa(limit)},
	}
	var w wireSearch
	if _, err := c.do(ctx, http.MethodGet, "/search", q, nil, &w); err != nil {
		return nil, err
	}

	res := &output.SearchResult{Query: query}
	if slices.Contains(types, "track") {
		out := []output.Item{}
		if w.Tracks != nil {
			for _, t := range w.Tracks.Items {
				if it := mapItem(t); it != nil && len(out) < limit {
					out = append(out, *it)
				}
			}
		}
		res.Tracks = &out
	}
	if slices.Contains(types, "album") {
		out := []output.AlbumEntry{}
		if w.Albums != nil {
			for _, a := range w.Albums.Items {
				if a != nil && len(out) < limit {
					out = append(out, mapAlbum(*a))
				}
			}
		}
		res.Albums = &out
	}
	if slices.Contains(types, "artist") {
		out := []output.ArtistEntry{}
		if w.Artists != nil {
			for _, a := range w.Artists.Items {
				if a != nil && len(out) < limit {
					out = append(out, mapArtistEntry(*a))
				}
			}
		}
		res.Artists = &out
	}
	if slices.Contains(types, "playlist") {
		out := []output.PlaylistEntry{}
		if w.Playlists != nil {
			for _, p := range w.Playlists.Items {
				if p != nil && len(out) < limit {
					out = append(out, mapPlaylist(*p))
				}
			}
		}
		res.Playlists = &out
	}
	return res, nil
}

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/Lazy-256/spotify-helper/internal/output"
	"github.com/Lazy-256/spotify-helper/internal/spotifyuri"
)

// Album returns the album u and one page of its tracks. Tracks of an album
// page are simplified objects without an album of their own, so each gets the
// album being read, and with it the album's cover.
func (c *Client) Album(ctx context.Context, u spotifyuri.URI, limit, offset int) (*output.AlbumContents, error) {
	var head wireAlbum
	if _, err := c.do(ctx, http.MethodGet, "/albums/"+url.PathEscape(u.ID), nil, nil, &head); err != nil {
		return nil, err
	}
	var page struct {
		Total int         `json:"total"`
		Items []*wireItem `json:"items"`
	}
	if _, err := c.do(ctx, http.MethodGet, "/albums/"+url.PathEscape(u.ID)+"/tracks", pageQuery(limit, offset), nil, &page); err != nil {
		return nil, err
	}

	res := &output.AlbumContents{
		Album:  mapAlbum(head),
		Tracks: []output.Item{},
		Total:  page.Total,
		Offset: offset,
		Limit:  limit,
	}
	for _, t := range page.Items {
		if t == nil {
			continue
		}
		if t.Type == "" {
			t.Type = "track"
		}
		t.Album = &head
		if it := mapItem(t); it != nil {
			res.Tracks = append(res.Tracks, *it)
		}
	}
	return res, nil
}

// Artist returns the artist u and one page of their albums and singles.
// Compilations and appearances on other artists' releases are not listed.
func (c *Client) Artist(ctx context.Context, u spotifyuri.URI, limit, offset int) (*output.ArtistContents, error) {
	var head wireArtistEntry
	if _, err := c.do(ctx, http.MethodGet, "/artists/"+url.PathEscape(u.ID), nil, nil, &head); err != nil {
		return nil, err
	}
	q := pageQuery(limit, offset)
	q.Set("include_groups", "album,single")
	var page struct {
		Total int          `json:"total"`
		Items []*wireAlbum `json:"items"`
	}
	if _, err := c.do(ctx, http.MethodGet, "/artists/"+url.PathEscape(u.ID)+"/albums", q, nil, &page); err != nil {
		return nil, err
	}

	res := &output.ArtistContents{
		Artist: mapArtistEntry(head),
		Albums: []output.AlbumEntry{},
		Total:  page.Total,
		Offset: offset,
		Limit:  limit,
	}
	for _, a := range page.Items {
		if a != nil {
			res.Albums = append(res.Albums, mapAlbum(*a))
		}
	}
	return res, nil
}

// wirePlaylistDetail is a playlist with the parts the playlist command reads.
// Since February 2026 the track listing is called items; before it was
// tracks. Either one, when present, means Spotify is willing to serve the
// playlist's items.
type wirePlaylistDetail struct {
	wirePlaylist
	Collaborative bool            `json:"collaborative"`
	Items         json.RawMessage `json:"items"`
	Tracks        json.RawMessage `json:"tracks"`
}

func present(raw json.RawMessage) bool {
	raw = bytes.TrimSpace(raw)
	return len(raw) > 0 && !bytes.Equal(raw, []byte("null"))
}

// wirePlaylistEntry is one entry of a playlist's items. The object sits under
// item (current) or track (older answers); local files and removed entries
// come back without a URI or as null.
type wirePlaylistEntry struct {
	IsLocal bool      `json:"is_local"`
	Item    *wireItem `json:"item"`
	Track   *wireItem `json:"track"`
}

// Playlist returns the playlist u and, when Spotify allows it, one page of its
// items. Spotify serves the items only of playlists the user owns or
// collaborates on: for any other playlist the answer has no items, or the
// items request is refused with 403, and TracksAvailable is false.
func (c *Client) Playlist(ctx context.Context, u spotifyuri.URI, limit, offset int) (*output.PlaylistContents, error) {
	var head wirePlaylistDetail
	if _, err := c.do(ctx, http.MethodGet, "/playlists/"+url.PathEscape(u.ID), nil, nil, &head); err != nil {
		return nil, err
	}
	res := &output.PlaylistContents{
		Playlist: output.PlaylistHeader{PlaylistEntry: mapPlaylist(head.wirePlaylist), Collaborative: head.Collaborative},
		Tracks:   []output.Item{},
		Offset:   offset,
		Limit:    limit,
	}
	if !present(head.Items) && !present(head.Tracks) {
		return res, nil
	}

	q := pageQuery(limit, offset)
	q.Set("additional_types", "track,episode")
	var page struct {
		Total int                  `json:"total"`
		Items []*wirePlaylistEntry `json:"items"`
	}
	status, err := c.do(ctx, http.MethodGet, "/playlists/"+url.PathEscape(u.ID)+"/items", q, nil, &page)
	if status == http.StatusForbidden && err != nil {
		return res, nil
	}
	if err != nil {
		return nil, err
	}

	res.TracksAvailable = true
	res.Total = page.Total
	for _, e := range page.Items {
		if e == nil || e.IsLocal {
			continue
		}
		w := e.Item
		if w == nil {
			w = e.Track
		}
		if w == nil || w.URI == "" {
			continue
		}
		if it := mapItem(w); it != nil {
			res.Tracks = append(res.Tracks, *it)
		}
	}
	return res, nil
}

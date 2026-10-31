package api

import "github.com/Lazy-256/spotify-helper/internal/output"

// Wire types: only the parts of Spotify's JSON that we read. Everything the
// plugin sees goes through the mappers below, so a change on Spotify's side
// cannot silently change the output contract.

type wireImage struct {
	URL   string `json:"url"`
	Width *int   `json:"width"`
}

type wireArtist struct {
	Name string `json:"name"`
	URI  string `json:"uri"`
}

type wireAlbum struct {
	ID          string       `json:"id"`
	URI         string       `json:"uri"`
	Name        string       `json:"name"`
	Artists     []wireArtist `json:"artists"`
	Images      []wireImage  `json:"images"`
	ReleaseDate string       `json:"release_date"`
	TotalTracks int          `json:"total_tracks"`
}

type wireShow struct {
	URI    string      `json:"uri"`
	Name   string      `json:"name"`
	Images []wireImage `json:"images"`
}

// wireItem is a track or an episode.
type wireItem struct {
	Type       string       `json:"type"`
	ID         string       `json:"id"`
	URI        string       `json:"uri"`
	Name       string       `json:"name"`
	DurationMS int64        `json:"duration_ms"`
	Explicit   bool         `json:"explicit"`
	IsPlayable *bool        `json:"is_playable"` // set only when the request has a market
	Artists    []wireArtist `json:"artists"`
	Album      *wireAlbum   `json:"album"`  // tracks
	Show       *wireShow    `json:"show"`   // episodes
	Images     []wireImage  `json:"images"` // episodes
}

type wireArtistEntry struct {
	ID     string      `json:"id"`
	URI    string      `json:"uri"`
	Name   string      `json:"name"`
	Images []wireImage `json:"images"`
}

type wirePlaylist struct {
	ID     string      `json:"id"`
	URI    string      `json:"uri"`
	Name   string      `json:"name"`
	Images []wireImage `json:"images"`
	Owner  *struct {
		DisplayName string `json:"display_name"`
		ID          string `json:"id"`
	} `json:"owner"`
}

// coverWidth is the width a cover should have at least; smaller covers look
// blurry at the sizes the widget draws them.
const coverWidth = 300

// coverURL picks the smallest image at least coverWidth wide, otherwise the
// largest one, otherwise nil. Images without a known width count as width 0;
// when no width is known at all the first image is used.
func coverURL(images []wireImage) *string {
	var best *wireImage
	bestWidth := -1
	for i := range images {
		img := &images[i]
		if img.URL == "" {
			continue
		}
		w := 0
		if img.Width != nil {
			w = *img.Width
		}
		switch {
		case best == nil:
			best, bestWidth = img, w
		case bestWidth >= coverWidth && w >= coverWidth && w < bestWidth:
			best, bestWidth = img, w // smaller of the large-enough
		case bestWidth < coverWidth && w > bestWidth:
			best, bestWidth = img, w // larger of the too-small
		}
	}
	if best == nil {
		return nil
	}
	u := best.URL
	return &u
}

func mapArtists(in []wireArtist) []output.Artist {
	out := make([]output.Artist, 0, len(in))
	for _, a := range in {
		out = append(out, output.Artist{Name: a.Name, URI: a.URI})
	}
	return out
}

// mapItem converts a track or episode; anything else (an advertisement, an
// unknown type, null) yields nil.
func mapItem(w *wireItem) *output.Item {
	if w == nil {
		return nil
	}
	it := &output.Item{
		Type:       w.Type,
		URI:        w.URI,
		ID:         w.ID,
		Name:       w.Name,
		DurationMS: w.DurationMS,
		Explicit:   w.Explicit,
	}
	switch w.Type {
	case "track":
		it.Artists = mapArtists(w.Artists)
		if w.Album != nil {
			it.Album = output.Ref{Name: w.Album.Name, URI: w.Album.URI}
			it.URL = coverURL(w.Album.Images)
		}
	case "episode":
		it.Artists = []output.Artist{}
		it.URL = coverURL(w.Images)
		if w.Show != nil {
			it.Album = output.Ref{Name: w.Show.Name, URI: w.Show.URI}
			if it.URL == nil {
				it.URL = coverURL(w.Show.Images)
			}
		}
	default:
		return nil
	}
	return it
}

func mapAlbum(w wireAlbum) output.AlbumEntry {
	e := output.AlbumEntry{
		URI: w.URI, ID: w.ID, Name: w.Name,
		Artists:     mapArtists(w.Artists),
		ReleaseDate: w.ReleaseDate,
		TotalTracks: w.TotalTracks,
	}
	e.URL = coverURL(w.Images)
	return e
}

func mapArtistEntry(w wireArtistEntry) output.ArtistEntry {
	e := output.ArtistEntry{URI: w.URI, ID: w.ID, Name: w.Name}
	e.URL = coverURL(w.Images)
	return e
}

func mapPlaylist(w wirePlaylist) output.PlaylistEntry {
	e := output.PlaylistEntry{URI: w.URI, ID: w.ID, Name: w.Name}
	e.URL = coverURL(w.Images)
	if w.Owner != nil {
		e.OwnerName = w.Owner.DisplayName
		if e.OwnerName == "" {
			e.OwnerName = w.Owner.ID
		}
	}
	return e
}

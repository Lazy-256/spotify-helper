package output

import "encoding/json"

// The types below are the JSON contract with the plugin. Field names and
// types are stable; new fields may be added (see the output-contract spec).

// CoverPath is the optional local path of a cached cover. It is absent from
// the JSON unless covers were requested, and then it is either a path or null.
type CoverPath struct {
	requested bool
	path      string
}

// Set records the outcome of a cover request; an empty path means none.
func (c *CoverPath) Set(path string) { c.requested, c.path = true, path }

// IsZero makes `omitzero` drop the field when covers were not requested.
func (c CoverPath) IsZero() bool { return !c.requested }

// Path returns the recorded path ("" when none).
func (c CoverPath) Path() string { return c.path }

func (c CoverPath) MarshalJSON() ([]byte, error) {
	if c.path == "" {
		return []byte("null"), nil
	}
	return json.Marshal(c.path)
}

// Cover is embedded in every entry that has an image.
type Cover struct {
	URL  *string   `json:"cover_url"`
	Path CoverPath `json:"cover_path,omitzero"`
}

// CoverRef lets the covers package reach the field through any entry type.
func (c *Cover) CoverRef() *Cover { return c }

// Coverable is implemented by every entry type that embeds Cover.
type Coverable interface{ CoverRef() *Cover }

// Artist is a name with its URI.
type Artist struct {
	Name string `json:"name"`
	URI  string `json:"uri"`
}

// Ref names an album or show.
type Ref struct {
	Name string `json:"name"`
	URI  string `json:"uri"`
}

// Item is a track or an episode. For an episode Artists is empty and Album
// describes the show.
type Item struct {
	Type       string   `json:"type"`
	URI        string   `json:"uri"`
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Artists    []Artist `json:"artists"`
	Album      Ref      `json:"album"`
	DurationMS int64    `json:"duration_ms"`
	Explicit   bool     `json:"explicit"`
	Cover
}

// AlbumEntry is an album in search results or the library.
type AlbumEntry struct {
	URI         string   `json:"uri"`
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Artists     []Artist `json:"artists"`
	ReleaseDate string   `json:"release_date"`
	TotalTracks int      `json:"total_tracks"`
	Cover
}

// ArtistEntry is an artist in search results.
type ArtistEntry struct {
	URI  string `json:"uri"`
	ID   string `json:"id"`
	Name string `json:"name"`
	Cover
}

// PlaylistEntry is a playlist in search results or the library.
type PlaylistEntry struct {
	URI       string `json:"uri"`
	ID        string `json:"id"`
	Name      string `json:"name"`
	OwnerName string `json:"owner_name"`
	Cover
}

// Device is a Spotify Connect device. State carries the active one; the
// devices command lists all of them.
type Device struct {
	ID            *string `json:"id"`
	Name          string  `json:"name"`
	Type          string  `json:"type"`
	IsActive      bool    `json:"is_active"`
	IsRestricted  bool    `json:"is_restricted"`
	VolumePercent *int    `json:"volume_percent"`
}

// PlaybackContext is the album, playlist or artist being played.
type PlaybackContext struct {
	Type string `json:"type"`
	URI  string `json:"uri"`
}

// State is the playback snapshot returned by `state`.
type State struct {
	Active      bool             `json:"active"`
	IsPlaying   bool             `json:"is_playing"`
	ProgressMS  int64            `json:"progress_ms"`
	FetchedAtMS int64            `json:"fetched_at_ms"`
	Shuffle     bool             `json:"shuffle"`
	Repeat      string           `json:"repeat"`
	Device      *Device          `json:"device"`
	Context     *PlaybackContext `json:"context"`
	Item        *Item            `json:"item"`
}

// SearchResult holds one array per requested type; types that were not
// requested are absent, requested ones are arrays even when empty.
type SearchResult struct {
	Query     string           `json:"query"`
	Tracks    *[]Item          `json:"tracks,omitempty"`
	Albums    *[]AlbumEntry    `json:"albums,omitempty"`
	Artists   *[]ArtistEntry   `json:"artists,omitempty"`
	Playlists *[]PlaylistEntry `json:"playlists,omitempty"`
}

// Covers returns every cover in the result, for the covers post-processing.
func (r *SearchResult) Covers() []*Cover {
	var out []*Cover
	if r.Tracks != nil {
		for i := range *r.Tracks {
			out = append(out, &(*r.Tracks)[i].Cover)
		}
	}
	if r.Albums != nil {
		for i := range *r.Albums {
			out = append(out, &(*r.Albums)[i].Cover)
		}
	}
	if r.Artists != nil {
		for i := range *r.Artists {
			out = append(out, &(*r.Artists)[i].Cover)
		}
	}
	if r.Playlists != nil {
		for i := range *r.Playlists {
			out = append(out, &(*r.Playlists)[i].Cover)
		}
	}
	return out
}

// QueueResult is the playback queue: what plays now and what comes next.
type QueueResult struct {
	NowPlaying *Item  `json:"now_playing"`
	Queue      []Item `json:"queue"`
}

// QueueAddAllResult is the `queue-add-all` command. Total is the number of
// items Spotify reports for the container, Skipped the examined entries that
// cannot be queued (local files, entries without a URI), and Truncated is true
// when the cap stopped the run with entries left unexamined.
type QueueAddAllResult struct {
	OK        bool   `json:"ok"`
	URI       string `json:"uri"`
	Added     int    `json:"added"`
	Skipped   int    `json:"skipped"`
	Total     int    `json:"total"`
	Offset    int    `json:"offset"`
	Truncated bool   `json:"truncated"`
}

// Covers returns every cover in the queue.
func (q *QueueResult) Covers() []*Cover {
	var out []*Cover
	if q.NowPlaying != nil {
		out = append(out, &q.NowPlaying.Cover)
	}
	for i := range q.Queue {
		out = append(out, &q.Queue[i].Cover)
	}
	return out
}

// PlaylistPage is one page of the user's playlists.
type PlaylistPage struct {
	Total     int             `json:"total"`
	Offset    int             `json:"offset"`
	Limit     int             `json:"limit"`
	Playlists []PlaylistEntry `json:"playlists"`
}

// Covers returns every cover on the page.
func (p *PlaylistPage) Covers() []*Cover {
	out := make([]*Cover, 0, len(p.Playlists))
	for i := range p.Playlists {
		out = append(out, &p.Playlists[i].Cover)
	}
	return out
}

// AlbumPage is one page of the user's saved albums.
type AlbumPage struct {
	Total  int          `json:"total"`
	Offset int          `json:"offset"`
	Limit  int          `json:"limit"`
	Albums []AlbumEntry `json:"albums"`
}

// Covers returns every cover on the page.
func (p *AlbumPage) Covers() []*Cover {
	out := make([]*Cover, 0, len(p.Albums))
	for i := range p.Albums {
		out = append(out, &p.Albums[i].Cover)
	}
	return out
}

// AlbumContents is the `album` command: the album and one page of its tracks.
type AlbumContents struct {
	Album  AlbumEntry `json:"album"`
	Tracks []Item     `json:"tracks"`
	Total  int        `json:"total"`
	Offset int        `json:"offset"`
	Limit  int        `json:"limit"`
}

// Covers returns the album's cover and the cover of every track.
func (a *AlbumContents) Covers() []*Cover {
	out := make([]*Cover, 0, len(a.Tracks)+1)
	out = append(out, &a.Album.Cover)
	for i := range a.Tracks {
		out = append(out, &a.Tracks[i].Cover)
	}
	return out
}

// PlaylistHeader is a playlist entry that also says whether others may edit
// it, so a caller can tell "not allowed" from "log in again".
type PlaylistHeader struct {
	PlaylistEntry
	Collaborative bool `json:"collaborative"`
}

// PlaylistContents is the `playlist` command. TracksAvailable is false when
// Spotify gave only the playlist's metadata; Tracks is then empty.
type PlaylistContents struct {
	Playlist        PlaylistHeader `json:"playlist"`
	TracksAvailable bool           `json:"tracks_available"`
	Tracks          []Item         `json:"tracks"`
	Total           int            `json:"total"`
	Offset          int            `json:"offset"`
	Limit           int            `json:"limit"`
}

// Covers returns the playlist's cover and the cover of every track.
func (p *PlaylistContents) Covers() []*Cover {
	out := make([]*Cover, 0, len(p.Tracks)+1)
	out = append(out, &p.Playlist.Cover)
	for i := range p.Tracks {
		out = append(out, &p.Tracks[i].Cover)
	}
	return out
}

// PlayedItem is one entry of the listening history: a track, when it was
// played (RFC 3339) and the context it was played from, if any.
type PlayedItem struct {
	PlayedAt string           `json:"played_at"`
	Context  *PlaybackContext `json:"context"`
	Track    Item             `json:"track"`
}

// Cursors are Spotify's time cursors of the history, in Unix milliseconds:
// Before is the value for the next older page, After for newer items. Either is
// null when there is no such page.
type Cursors struct {
	Before *int64 `json:"before"`
	After  *int64 `json:"after"`
}

// RecentPage is the `recent` command.
type RecentPage struct {
	Limit   int          `json:"limit"`
	Cursors Cursors      `json:"cursors"`
	Items   []PlayedItem `json:"items"`
}

// Covers returns the cover of every played track.
func (r *RecentPage) Covers() []*Cover {
	out := make([]*Cover, 0, len(r.Items))
	for i := range r.Items {
		out = append(out, &r.Items[i].Track.Cover)
	}
	return out
}

// TopResult is the `top` command. Exactly one of Tracks and Artists is set,
// as a pointer so that an empty list still appears as [].
type TopResult struct {
	Type    string         `json:"type"`
	Range   string         `json:"range"`
	Total   int            `json:"total"`
	Offset  int            `json:"offset"`
	Limit   int            `json:"limit"`
	Tracks  *[]Item        `json:"tracks,omitempty"`
	Artists *[]ArtistEntry `json:"artists,omitempty"`
}

// Covers returns every cover in the result.
func (t *TopResult) Covers() []*Cover {
	var out []*Cover
	if t.Tracks != nil {
		for i := range *t.Tracks {
			out = append(out, &(*t.Tracks)[i].Cover)
		}
	}
	if t.Artists != nil {
		for i := range *t.Artists {
			out = append(out, &(*t.Artists)[i].Cover)
		}
	}
	return out
}

// ArtistContents is the `artist` command: the artist and one page of their
// albums and singles.
type ArtistContents struct {
	Artist ArtistEntry  `json:"artist"`
	Albums []AlbumEntry `json:"albums"`
	Total  int          `json:"total"`
	Offset int          `json:"offset"`
	Limit  int          `json:"limit"`
}

// Covers returns the artist's cover and the cover of every album.
func (a *ArtistContents) Covers() []*Cover {
	out := make([]*Cover, 0, len(a.Albums)+1)
	out = append(out, &a.Artist.Cover)
	for i := range a.Albums {
		out = append(out, &a.Albums[i].Cover)
	}
	return out
}

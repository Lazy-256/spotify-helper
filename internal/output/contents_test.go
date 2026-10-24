package output

import (
	"bytes"
	"testing"
)

func str(s string) *string { return &s }

func marshal(t *testing.T, v any) string {
	t.Helper()
	var buf bytes.Buffer
	if err := WriteJSON(&buf, v); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestAlbumContentsJSON(t *testing.T) {
	a := &AlbumContents{
		Album: AlbumEntry{URI: "spotify:album:a", ID: "a", Name: "A", Artists: []Artist{{Name: "X", URI: "spotify:artist:x"}},
			ReleaseDate: "2020", TotalTracks: 1, Cover: Cover{URL: str("https://i/a")}},
		Tracks: []Item{{Type: "track", URI: "spotify:track:t", ID: "t", Name: "T", Artists: []Artist{},
			Album: Ref{Name: "A", URI: "spotify:album:a"}, DurationMS: 5, Cover: Cover{URL: str("https://i/a")}}},
		Total: 1, Offset: 0, Limit: 50,
	}
	want := `{"album":{"uri":"spotify:album:a","id":"a","name":"A","artists":[{"name":"X","uri":"spotify:artist:x"}],` +
		`"release_date":"2020","total_tracks":1,"cover_url":"https://i/a"},` +
		`"tracks":[{"type":"track","uri":"spotify:track:t","id":"t","name":"T","artists":[],` +
		`"album":{"name":"A","uri":"spotify:album:a"},"duration_ms":5,"explicit":false,"cover_url":"https://i/a"}],` +
		`"total":1,"offset":0,"limit":50}` + "\n"
	if got := marshal(t, a); got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}

	// Once covers were requested every cover, header included, carries cover_path.
	for _, c := range a.Covers() {
		c.Path.Set("/cache/a.jpg")
	}
	got := marshal(t, a)
	if n := bytes.Count([]byte(got), []byte(`"cover_path":"/cache/a.jpg"`)); n != 2 {
		t.Errorf("want cover_path on the album and its track, got %d in %s", n, got)
	}
}

func TestPlaylistContentsJSON(t *testing.T) {
	p := &PlaylistContents{
		Playlist: PlaylistHeader{
			PlaylistEntry: PlaylistEntry{URI: "spotify:playlist:p", ID: "p", Name: "P", OwnerName: "me"},
			Collaborative: true,
		},
		Tracks: []Item{},
		Limit:  50,
	}
	want := `{"playlist":{"uri":"spotify:playlist:p","id":"p","name":"P","owner_name":"me","cover_url":null,"collaborative":true},` +
		`"tracks_available":false,"tracks":[],"total":0,"offset":0,"limit":50}` + "\n"
	if got := marshal(t, p); got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}

	p.Playlist.Cover.Path.Set("")
	want = `{"playlist":{"uri":"spotify:playlist:p","id":"p","name":"P","owner_name":"me","cover_url":null,"cover_path":null,"collaborative":true},` +
		`"tracks_available":false,"tracks":[],"total":0,"offset":0,"limit":50}` + "\n"
	if got := marshal(t, p); got != want {
		t.Errorf("with covers requested:\ngot  %s\nwant %s", got, want)
	}
	if n := len(p.Covers()); n != 1 {
		t.Errorf("want only the playlist cover, got %d", n)
	}
}

func TestArtistContentsJSON(t *testing.T) {
	a := &ArtistContents{
		Artist: ArtistEntry{URI: "spotify:artist:x", ID: "x", Name: "X"},
		Albums: []AlbumEntry{},
		Limit:  50,
	}
	want := `{"artist":{"uri":"spotify:artist:x","id":"x","name":"X","cover_url":null},"albums":[],"total":0,"offset":0,"limit":50}` + "\n"
	if got := marshal(t, a); got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
	a.Albums = append(a.Albums, AlbumEntry{URI: "spotify:album:a"})
	if n := len(a.Covers()); n != 2 {
		t.Errorf("want artist and album covers, got %d", n)
	}
}

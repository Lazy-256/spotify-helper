package spotifyuri

import (
	"errors"
	"testing"

	"github.com/Lazy-256/spotify-helper/internal/output"
)

func TestParse(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"spotify:track:abc", "spotify:track:abc"},
		{"spotify:album:2noRn2Aes5aoNVsU6iWThc", "spotify:album:2noRn2Aes5aoNVsU6iWThc"},
		{"  spotify:playlist:0jcBmnxCJ0oFZRsEGpYlCM \n", "spotify:playlist:0jcBmnxCJ0oFZRsEGpYlCM"},
		{"https://open.spotify.com/track/abc", "spotify:track:abc"},
		{"https://open.spotify.com/track/abc?si=xyz", "spotify:track:abc"},
		{"https://open.spotify.com/intl-nl/album/abc?si=1", "spotify:album:abc"},
		{"https://open.spotify.com/intl-pt-br/artist/abc", "spotify:artist:abc"},
		{"http://open.spotify.com/episode/abc/", "spotify:episode:abc"},
		{"https://open.spotify.com/show/abc", "spotify:show:abc"},
		{"spotify:artist:0TnOYISbd1XYRBk9myaseg", "spotify:artist:0TnOYISbd1XYRBk9myaseg"},
		{"https://open.spotify.com/album/abc?si=xyz", "spotify:album:abc"},
		{"https://open.spotify.com/intl-de/album/abc?si=xyz", "spotify:album:abc"},
		{"https://open.spotify.com/playlist/37i9dQZF1DXcBWIGoYBM5M?si=1", "spotify:playlist:37i9dQZF1DXcBWIGoYBM5M"},
		{"https://open.spotify.com/intl-pt-br/playlist/abc", "spotify:playlist:abc"},
		{"https://open.spotify.com/artist/abc?si=xyz", "spotify:artist:abc"},
	}
	for _, tt := range tests {
		got, err := Parse(tt.in)
		if err != nil {
			t.Errorf("Parse(%q): %v", tt.in, err)
			continue
		}
		if got.String() != tt.want {
			t.Errorf("Parse(%q) = %s, want %s", tt.in, got, tt.want)
		}
	}
}

func TestParseRejects(t *testing.T) {
	for _, in := range []string{
		"", "hello", "spotify:track", "spotify:track:", "spotify:track:a:b",
		"spotify:user:abc", "spotify:track:a b", "spotify:track:ab-c",
		"https://example.com/track/abc", "https://open.spotify.com/",
		"https://open.spotify.com/track", "https://open.spotify.com/track/abc/extra",
		"https://open.spotify.com/user/abc", "ftp://open.spotify.com/track/abc",
	} {
		_, err := Parse(in)
		var oe *output.Error
		if !errors.As(err, &oe) || oe.Code != output.CodeUsage {
			t.Errorf("Parse(%q): want usage error, got %v", in, err)
		}
	}
}

func TestParseAllowed(t *testing.T) {
	if u, err := ParseAllowed("spotify:track:abc", Track, Episode); err != nil || u.Type != Track {
		t.Errorf("track should be allowed: %v %v", u, err)
	}
	_, err := ParseAllowed("spotify:album:abc", Track, Episode)
	var oe *output.Error
	if !errors.As(err, &oe) || oe.Code != output.CodeUsage {
		t.Errorf("album should be rejected with usage, got %v", err)
	}
}

func TestIsContext(t *testing.T) {
	for typ, want := range map[Type]bool{
		Track: false, Episode: false, Album: true, Playlist: true, Artist: true, Show: true,
	} {
		if got := (URI{Type: typ, ID: "x"}).IsContext(); got != want {
			t.Errorf("%s IsContext = %v, want %v", typ, got, want)
		}
	}
}

// The catalog commands accept exactly one of album, playlist and artist each.
func TestParseAllowedCatalogTypes(t *testing.T) {
	cases := []struct {
		allowed Type
		in      string
		want    string
	}{
		{Album, "spotify:album:abc", "spotify:album:abc"},
		{Album, "https://open.spotify.com/intl-nl/album/abc?si=x", "spotify:album:abc"},
		{Playlist, "spotify:playlist:abc", "spotify:playlist:abc"},
		{Playlist, "https://open.spotify.com/playlist/abc?si=x", "spotify:playlist:abc"},
		{Artist, "spotify:artist:abc", "spotify:artist:abc"},
		{Artist, "https://open.spotify.com/intl-pt-br/artist/abc?si=x", "spotify:artist:abc"},
	}
	for _, tt := range cases {
		u, err := ParseAllowed(tt.in, tt.allowed)
		if err != nil || u.String() != tt.want {
			t.Errorf("ParseAllowed(%q, %s) = %v, %v; want %s", tt.in, tt.allowed, u, err, tt.want)
		}
	}

	for _, allowed := range []Type{Album, Playlist, Artist} {
		for _, in := range []string{
			"spotify:track:abc", "spotify:episode:abc", "spotify:show:abc",
			"https://open.spotify.com/track/abc", "hello",
		} {
			_, err := ParseAllowed(in, allowed)
			var oe *output.Error
			if !errors.As(err, &oe) || oe.Code != output.CodeUsage {
				t.Errorf("ParseAllowed(%q, %s): want usage error, got %v", in, allowed, err)
			}
		}
	}
	// ...and not each other.
	for _, tt := range [][2]string{
		{"spotify:playlist:abc", string(Album)},
		{"spotify:artist:abc", string(Album)},
		{"spotify:album:abc", string(Playlist)},
		{"spotify:album:abc", string(Artist)},
	} {
		if _, err := ParseAllowed(tt[0], Type(tt[1])); err == nil {
			t.Errorf("%s should not be accepted as %s", tt[0], tt[1])
		}
	}
}

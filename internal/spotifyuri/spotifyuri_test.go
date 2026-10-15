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

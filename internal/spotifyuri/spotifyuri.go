// Package spotifyuri parses Spotify URIs and open.spotify.com links.
package spotifyuri

import (
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/Lazy-256/spotify-helper/internal/output"
)

// Type is the kind of Spotify item a URI points to.
type Type string

const (
	Track    Type = "track"
	Episode  Type = "episode"
	Album    Type = "album"
	Playlist Type = "playlist"
	Artist   Type = "artist"
	Show     Type = "show"
)

var known = map[Type]bool{
	Track: true, Episode: true, Album: true, Playlist: true, Artist: true, Show: true,
}

var (
	idPattern   = regexp.MustCompile(`^[0-9A-Za-z]+$`)
	intlSegment = regexp.MustCompile(`^intl-[A-Za-z]+(-[A-Za-z]+)?$`)
)

// URI is a normalized Spotify item reference.
type URI struct {
	Type Type
	ID   string
}

// String returns the canonical spotify:<type>:<id> form.
func (u URI) String() string { return "spotify:" + string(u.Type) + ":" + u.ID }

// IsContext reports whether the URI is played as a context (a list of tracks)
// rather than as a single item.
func (u URI) IsContext() bool {
	switch u.Type {
	case Album, Playlist, Artist, Show:
		return true
	}
	return false
}

// Parse accepts spotify:<type>:<id> and
// https://open.spotify.com/[intl-xx/]<type>/<id>[?…] and returns the
// normalized value. Failures are usage errors.
func Parse(s string) (URI, error) {
	s = strings.TrimSpace(s)
	switch {
	case strings.HasPrefix(s, "spotify:"):
		parts := strings.Split(s, ":")
		if len(parts) != 3 {
			return URI{}, bad(s)
		}
		return build(parts[1], parts[2], s)
	case strings.HasPrefix(s, "https://"), strings.HasPrefix(s, "http://"):
		u, err := url.Parse(s)
		if err != nil || u.Host != "open.spotify.com" {
			return URI{}, bad(s)
		}
		segs := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(segs) > 0 && intlSegment.MatchString(segs[0]) {
			segs = segs[1:]
		}
		if len(segs) != 2 {
			return URI{}, bad(s)
		}
		return build(segs[0], segs[1], s)
	}
	return URI{}, bad(s)
}

// ParseAllowed is Parse restricted to the given types.
func ParseAllowed(s string, allowed ...Type) (URI, error) {
	u, err := Parse(s)
	if err != nil {
		return URI{}, err
	}
	if slices.Contains(allowed, u.Type) {
		return u, nil
	}
	names := make([]string, len(allowed))
	for i, t := range allowed {
		names[i] = string(t)
	}
	return URI{}, output.Usage("%s is of type %s; this command accepts only: %s", u, u.Type, strings.Join(names, ", "))
}

func build(typ, id, orig string) (URI, error) {
	t := Type(typ)
	if !known[t] || !idPattern.MatchString(id) {
		return URI{}, bad(orig)
	}
	return URI{Type: t, ID: id}, nil
}

func bad(s string) error {
	return output.Usage("%q is not a Spotify URI or open.spotify.com link", s)
}

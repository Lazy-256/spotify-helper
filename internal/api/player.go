package api

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"github.com/Lazy-256/spotify-helper/internal/output"
	"github.com/Lazy-256/spotify-helper/internal/spotifyuri"
)

type wireDevice struct {
	ID            *string `json:"id"`
	Name          string  `json:"name"`
	Type          string  `json:"type"`
	IsActive      bool    `json:"is_active"`
	IsRestricted  bool    `json:"is_restricted"`
	VolumePercent *int    `json:"volume_percent"`
}

func mapDevice(w wireDevice) output.Device {
	return output.Device{
		ID: w.ID, Name: w.Name, Type: w.Type,
		IsActive: w.IsActive, IsRestricted: w.IsRestricted, VolumePercent: w.VolumePercent,
	}
}

type wirePlayer struct {
	Device       *wireDevice `json:"device"`
	RepeatState  string      `json:"repeat_state"`
	ShuffleState bool        `json:"shuffle_state"`
	Context      *struct {
		Type string `json:"type"`
		URI  string `json:"uri"`
	} `json:"context"`
	ProgressMS int64     `json:"progress_ms"`
	IsPlaying  bool      `json:"is_playing"`
	Item       *wireItem `json:"item"`
}

// State returns the current playback. Nothing active is not an error: the
// result then has Active false and null device, context and item.
func (c *Client) State(ctx context.Context) (*output.State, error) {
	var p wirePlayer
	q := url.Values{"additional_types": {"track,episode"}}
	status, err := c.do(ctx, http.MethodGet, "/me/player", q, nil, &p)
	if err != nil {
		return nil, err
	}
	st := &output.State{FetchedAtMS: c.Now().UnixMilli(), Repeat: "off"}
	if status == http.StatusNoContent || p.Device == nil {
		return st, nil
	}
	st.Active = true
	st.IsPlaying = p.IsPlaying
	st.ProgressMS = p.ProgressMS
	st.Shuffle = p.ShuffleState
	if p.RepeatState != "" {
		st.Repeat = p.RepeatState
	}
	d := mapDevice(*p.Device)
	st.Device = &d
	if p.Context != nil {
		st.Context = &output.PlaybackContext{Type: p.Context.Type, URI: p.Context.URI}
	}
	st.Item = mapItem(p.Item)
	return st, nil
}

func deviceQuery(device string, extra url.Values) url.Values {
	q := url.Values{}
	for k, v := range extra {
		q[k] = v
	}
	if device != "" {
		q.Set("device_id", device)
	}
	return q
}

// Play resumes the current playback.
func (c *Client) Play(ctx context.Context, device string) error {
	_, err := c.do(ctx, http.MethodPut, "/me/player/play", deviceQuery(device, nil), nil, nil)
	return err
}

// Pause pauses the current playback.
func (c *Client) Pause(ctx context.Context, device string) error {
	_, err := c.do(ctx, http.MethodPut, "/me/player/pause", deviceQuery(device, nil), nil, nil)
	return err
}

// Next skips to the next item.
func (c *Client) Next(ctx context.Context, device string) error {
	_, err := c.do(ctx, http.MethodPost, "/me/player/next", deviceQuery(device, nil), nil, nil)
	return err
}

// Previous goes back to the previous item.
func (c *Client) Previous(ctx context.Context, device string) error {
	_, err := c.do(ctx, http.MethodPost, "/me/player/previous", deviceQuery(device, nil), nil, nil)
	return err
}

// SetVolume sets the volume to percent (0–100).
func (c *Client) SetVolume(ctx context.Context, percent int, device string) error {
	q := deviceQuery(device, url.Values{"volume_percent": {strconv.Itoa(percent)}})
	_, err := c.do(ctx, http.MethodPut, "/me/player/volume", q, nil, nil)
	return err
}

// Seek moves to positionMS in the current item.
func (c *Client) Seek(ctx context.Context, positionMS int64, device string) error {
	q := deviceQuery(device, url.Values{"position_ms": {strconv.FormatInt(positionMS, 10)}})
	_, err := c.do(ctx, http.MethodPut, "/me/player/seek", q, nil, nil)
	return err
}

// SetShuffle turns shuffle on or off.
func (c *Client) SetShuffle(ctx context.Context, on bool, device string) error {
	q := deviceQuery(device, url.Values{"state": {strconv.FormatBool(on)}})
	_, err := c.do(ctx, http.MethodPut, "/me/player/shuffle", q, nil, nil)
	return err
}

// SetRepeat sets repeat to off, context or track.
func (c *Client) SetRepeat(ctx context.Context, mode, device string) error {
	q := deviceQuery(device, url.Values{"state": {mode}})
	_, err := c.do(ctx, http.MethodPut, "/me/player/repeat", q, nil, nil)
	return err
}

// PlayURI starts playback of u: a track or episode as a single item, anything
// else as a context from its beginning.
func (c *Client) PlayURI(ctx context.Context, u spotifyuri.URI, device string) error {
	var body map[string]any
	if u.IsContext() {
		body = map[string]any{"context_uri": u.String()}
	} else {
		body = map[string]any{"uris": []string{u.String()}}
	}
	_, err := c.do(ctx, http.MethodPut, "/me/player/play", deviceQuery(device, nil), body, nil)
	return err
}

// PlayFrom starts the album or playlist context at item ("play from here").
// Spotify takes the position as an offset naming the item; it answers with an
// error when the item is not part of the context.
func (c *Client) PlayFrom(ctx context.Context, item, within spotifyuri.URI, device string) error {
	body := map[string]any{
		"context_uri": within.String(),
		"offset":      map[string]string{"uri": item.String()},
	}
	_, err := c.do(ctx, http.MethodPut, "/me/player/play", deviceQuery(device, nil), body, nil)
	return err
}

// Devices lists the user's Connect devices in Spotify's order.
func (c *Client) Devices(ctx context.Context) ([]output.Device, error) {
	var resp struct {
		Devices []wireDevice `json:"devices"`
	}
	if _, err := c.do(ctx, http.MethodGet, "/me/player/devices", nil, nil, &resp); err != nil {
		return nil, err
	}
	out := make([]output.Device, 0, len(resp.Devices))
	for _, d := range resp.Devices {
		out = append(out, mapDevice(d))
	}
	return out, nil
}

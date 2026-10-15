package cli

import (
	"context"
	"regexp"
	"strconv"

	"github.com/Lazy-256/spotify-helper/internal/api"
	"github.com/Lazy-256/spotify-helper/internal/output"
	"github.com/Lazy-256/spotify-helper/internal/spotifyuri"
)

func init() {
	register(&command{name: "state", usage: "[--covers]", summary: "show what is playing and on which device", run: runState})

	simple := func(name, summary string, do func(c *api.Client, ctx context.Context, device string) error) {
		register(&command{
			name: name, usage: "[--device ID]", summary: summary,
			run: func(env *Env, args []string) error {
				_, f, err := parseCommon(name, "[--device ID]", args, 0, 0, flagSet{device: true})
				if err != nil {
					return err
				}
				return withClient(env, func(ctx context.Context, c *api.Client) error {
					if err := do(c, ctx, f.device); err != nil {
						return err
					}
					return output.WriteOK(env.Stdout)
				})
			},
		})
	}
	simple("play", "resume playback", (*api.Client).Play)
	simple("pause", "pause playback", (*api.Client).Pause)
	simple("next", "skip to the next item", (*api.Client).Next)
	simple("prev", "go back to the previous item", (*api.Client).Previous)

	register(&command{name: "toggle", usage: "[--device ID]", summary: "pause when playing, resume otherwise", run: runToggle})
	register(&command{name: "volume", usage: "<0-100|+N|-N> [--device ID]", summary: "set or change the volume", run: runVolume})
	register(&command{name: "seek", usage: "<ms|+ms|-ms> [--device ID]", summary: "seek to a position or by an offset", run: runSeek})
	register(&command{name: "shuffle", usage: "<on|off|toggle> [--device ID]", summary: "set shuffle", run: runShuffle})
	register(&command{name: "repeat", usage: "<off|context|track|cycle> [--device ID]", summary: "set repeat mode", run: runRepeat})
	register(&command{name: "play-uri", usage: "<uri|link> [--device ID]", summary: "start playing a track, album, playlist, artist, episode or show", run: runPlayURI})
}

// withClient builds the API client and runs fn under the command timeout.
func withClient(env *Env, fn func(ctx context.Context, c *api.Client) error) error {
	c, _, err := env.apiClient()
	if err != nil {
		return err
	}
	ctx, cancel := opContext()
	defer cancel()
	return fn(ctx, c)
}

func noActiveDevice() error {
	return output.Errorf(output.CodeNoActiveDevice, "no active Spotify device; start playback on a device or pass --device")
}

func runState(env *Env, args []string) error {
	_, f, err := parseCommon("state", "[--covers]", args, 0, 0, flagSet{covers: true})
	if err != nil {
		return err
	}
	return withClient(env, func(ctx context.Context, c *api.Client) error {
		st, err := c.State(ctx)
		if err != nil {
			return err
		}
		if st.Item != nil {
			if err := env.attachCovers(ctx, f.covers, []*output.Cover{&st.Item.Cover}); err != nil {
				return err
			}
		}
		return output.WriteJSON(env.Stdout, st)
	})
}

func runToggle(env *Env, args []string) error {
	_, f, err := parseCommon("toggle", "[--device ID]", args, 0, 0, flagSet{device: true})
	if err != nil {
		return err
	}
	return withClient(env, func(ctx context.Context, c *api.Client) error {
		st, err := c.State(ctx)
		if err != nil {
			return err
		}
		if !st.Active && f.device == "" {
			return noActiveDevice()
		}
		// is_playing describes the active device; for another target there
		// is nothing to pause, so start it.
		onTarget := f.device == "" || (st.Device != nil && st.Device.ID != nil && *st.Device.ID == f.device)
		if st.IsPlaying && onTarget {
			err = c.Pause(ctx, f.device)
		} else {
			err = c.Play(ctx, f.device)
		}
		if err != nil {
			return err
		}
		return output.WriteOK(env.Stdout)
	})
}

var adjustPattern = regexp.MustCompile(`^[+-]?[0-9]+$`)

// adjust is a number that is either absolute ("40") or relative ("+5", "-10").
type adjust struct {
	value    int64
	relative bool
}

func parseAdjust(s string) (adjust, bool) {
	if !adjustPattern.MatchString(s) {
		return adjust{}, false
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return adjust{}, false
	}
	return adjust{value: v, relative: s[0] == '+' || s[0] == '-'}, true
}

func clamp[T int | int64](v, lo, hi T) T { return min(max(v, lo), hi) }

func runVolume(env *Env, args []string) error {
	pos, f, err := parseCommon("volume", "<0-100|+N|-N> [--device ID]", args, 1, 1, flagSet{device: true})
	if err != nil {
		return err
	}
	adj, ok := parseAdjust(pos[0])
	if !ok || (!adj.relative && adj.value > 100) {
		return output.Usage("volume must be 0-100, or a change such as +5 or -10, got %q", pos[0])
	}
	return withClient(env, func(ctx context.Context, c *api.Client) error {
		target := int(adj.value)
		if adj.relative {
			devices, err := c.Devices(ctx)
			if err != nil {
				return err
			}
			cur, found := -1, false
			for _, d := range devices {
				if (f.device != "" && d.ID != nil && *d.ID == f.device) || (f.device == "" && d.IsActive) {
					found = true
					if d.VolumePercent != nil {
						cur = *d.VolumePercent
					}
					break
				}
			}
			switch {
			case !found && f.device == "":
				return noActiveDevice()
			case !found:
				return output.Errorf(output.CodeDeviceNotFound, "no device with id %q", f.device)
			case cur < 0:
				return output.Errorf(output.CodeAPIError, "the device does not report its volume, so it cannot be changed relatively")
			}
			target = cur + int(clamp(adj.value, -100, 100))
		}
		target = clamp(target, 0, 100)
		if err := c.SetVolume(ctx, target, f.device); err != nil {
			return err
		}
		return output.WriteJSON(env.Stdout, struct {
			OK            bool `json:"ok"`
			VolumePercent int  `json:"volume_percent"`
		}{true, target})
	})
}

// seekEndMargin keeps a seek one second short of the end of an item. A seek to
// the very end finishes the track, which can leave a Connect device in a stuck
// end-of-track session (seen on a hifiberry running spotify-soloist).
const seekEndMargin = 1000

func runSeek(env *Env, args []string) error {
	pos, f, err := parseCommon("seek", "<ms|+ms|-ms> [--device ID]", args, 1, 1, flagSet{device: true})
	if err != nil {
		return err
	}
	adj, ok := parseAdjust(pos[0])
	if !ok || (!adj.relative && adj.value < 0) {
		return output.Usage("seek takes a position in milliseconds or an offset such as +10000 or -5000, got %q", pos[0])
	}
	return withClient(env, func(ctx context.Context, c *api.Client) error {
		st, err := c.State(ctx)
		if err != nil {
			return err
		}
		if !st.Active && f.device == "" {
			return noActiveDevice()
		}
		target := adj.value
		if adj.relative {
			target += st.ProgressMS
		}
		target = max(target, 0)
		if st.Item != nil && st.Item.DurationMS > 0 {
			target = min(target, max(st.Item.DurationMS-seekEndMargin, 0))
		}
		if err := c.Seek(ctx, target, f.device); err != nil {
			return err
		}
		return output.WriteJSON(env.Stdout, struct {
			OK         bool  `json:"ok"`
			PositionMS int64 `json:"position_ms"`
		}{true, target})
	})
}

func runShuffle(env *Env, args []string) error {
	pos, f, err := parseCommon("shuffle", "<on|off|toggle> [--device ID]", args, 1, 1, flagSet{device: true})
	if err != nil {
		return err
	}
	mode := pos[0]
	if mode != "on" && mode != "off" && mode != "toggle" {
		return output.Usage("shuffle takes on, off or toggle, got %q", mode)
	}
	return withClient(env, func(ctx context.Context, c *api.Client) error {
		on := mode == "on"
		if mode == "toggle" {
			st, err := c.State(ctx)
			if err != nil {
				return err
			}
			if !st.Active && f.device == "" {
				return noActiveDevice()
			}
			on = !st.Shuffle
		}
		if err := c.SetShuffle(ctx, on, f.device); err != nil {
			return err
		}
		return output.WriteJSON(env.Stdout, struct {
			OK      bool `json:"ok"`
			Shuffle bool `json:"shuffle"`
		}{true, on})
	})
}

func runRepeat(env *Env, args []string) error {
	pos, f, err := parseCommon("repeat", "<off|context|track|cycle> [--device ID]", args, 1, 1, flagSet{device: true})
	if err != nil {
		return err
	}
	mode := pos[0]
	switch mode {
	case "off", "context", "track", "cycle":
	default:
		return output.Usage("repeat takes off, context, track or cycle, got %q", mode)
	}
	return withClient(env, func(ctx context.Context, c *api.Client) error {
		if mode == "cycle" {
			st, err := c.State(ctx)
			if err != nil {
				return err
			}
			if !st.Active && f.device == "" {
				return noActiveDevice()
			}
			mode = map[string]string{"off": "context", "context": "track", "track": "off"}[st.Repeat]
			if mode == "" {
				mode = "off"
			}
		}
		if err := c.SetRepeat(ctx, mode, f.device); err != nil {
			return err
		}
		return output.WriteJSON(env.Stdout, struct {
			OK     bool   `json:"ok"`
			Repeat string `json:"repeat"`
		}{true, mode})
	})
}

func runPlayURI(env *Env, args []string) error {
	pos, f, err := parseCommon("play-uri", "<uri|link> [--device ID]", args, 1, 1, flagSet{device: true})
	if err != nil {
		return err
	}
	u, err := spotifyuri.ParseAllowed(pos[0],
		spotifyuri.Track, spotifyuri.Episode, spotifyuri.Album,
		spotifyuri.Playlist, spotifyuri.Artist, spotifyuri.Show)
	if err != nil {
		return err
	}
	return withClient(env, func(ctx context.Context, c *api.Client) error {
		if err := c.PlayURI(ctx, u, f.device); err != nil {
			return err
		}
		return output.WriteOK(env.Stdout)
	})
}

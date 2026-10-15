package cli

import (
	"context"
	"flag"
	"strings"

	"github.com/Lazy-256/spotify-helper/internal/api"
	"github.com/Lazy-256/spotify-helper/internal/output"
)

func init() {
	register(&command{name: "devices", usage: "", summary: "list Spotify Connect devices", run: runDevices})
	register(&command{name: "transfer", usage: "<device id or name> [--play]", summary: "move playback to another device", run: runTransfer})
}

func runDevices(env *Env, args []string) error {
	if _, _, err := parseCommon("devices", "", args, 0, 0, flagSet{}); err != nil {
		return err
	}
	return withClient(env, func(ctx context.Context, c *api.Client) error {
		devices, err := c.Devices(ctx)
		if err != nil {
			return err
		}
		return output.WriteJSON(env.Stdout, struct {
			Devices []output.Device `json:"devices"`
		}{devices})
	})
}

func runTransfer(env *Env, args []string) error {
	var play bool
	pos, _, err := parseCommon("transfer", "<device id or name> [--play]", args, 1, 1,
		flagSet{extra: func(fs *flag.FlagSet) { fs.BoolVar(&play, "play", false, "start playback on the target") }})
	if err != nil {
		return err
	}
	want := pos[0]
	return withClient(env, func(ctx context.Context, c *api.Client) error {
		devices, err := c.Devices(ctx)
		if err != nil {
			return err
		}
		target, err := resolveDevice(devices, want)
		if err != nil {
			return err
		}
		if err := c.Transfer(ctx, *target.ID, play); err != nil {
			return err
		}
		return output.WriteJSON(env.Stdout, struct {
			OK     bool `json:"ok"`
			Device struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"device"`
		}{OK: true, Device: struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}{*target.ID, target.Name}})
	})
}

// resolveDevice finds the device whose id equals want or, failing that,
// whose name equals it ignoring case. Restricted devices (which accept no
// remote commands) and devices without an id are never eligible.
func resolveDevice(devices []output.Device, want string) (output.Device, error) {
	var usable []output.Device
	for _, d := range devices {
		if !d.IsRestricted && d.ID != nil {
			usable = append(usable, d)
		}
	}
	for _, d := range usable {
		if *d.ID == want {
			return d, nil
		}
	}
	var byName []output.Device
	for _, d := range usable {
		if strings.EqualFold(d.Name, want) {
			byName = append(byName, d)
		}
	}
	switch len(byName) {
	case 1:
		return byName[0], nil
	case 0:
		return output.Device{}, output.Errorf(output.CodeDeviceNotFound, "no available device with id or name %q", want)
	default:
		return output.Device{}, output.Errorf(output.CodeAmbiguousDevice, "%d devices are named %q; use the device id", len(byName), want)
	}
}

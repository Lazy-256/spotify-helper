package cli

import "github.com/Lazy-256/spotify-helper/internal/output"

func init() {
	register(&command{
		name:    "version",
		usage:   "",
		summary: "print the helper version",
		run: func(env *Env, args []string) error {
			if len(args) != 0 {
				return output.Usage("usage: spotify-helper version")
			}
			return output.WriteJSON(env.Stdout, struct {
				Version string `json:"version"`
			}{Version})
		},
	})
}

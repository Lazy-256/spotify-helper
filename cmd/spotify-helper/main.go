// Command spotify-helper controls Spotify through the Web API and speaks JSON.
package main

import (
	"os"

	"github.com/Lazy-256/spotify-helper/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], cli.DefaultEnv()))
}

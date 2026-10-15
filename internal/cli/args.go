package cli

import (
	"flag"
	"io"
	"regexp"
	"strings"

	"github.com/Lazy-256/spotify-helper/internal/output"
)

var numeric = regexp.MustCompile(`^-[0-9]+(\.[0-9]+)?$`)

// parseArgs parses flags and positionals in any order, which the standard
// flag package does not. Negative numbers (`volume -10`, `seek -5000`) are
// positionals, and `--` ends flag parsing. Any parse problem is a usage error.
func parseArgs(fs *flag.FlagSet, args []string) ([]string, error) {
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}

	var flags, pos []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			pos = append(pos, args[i+1:]...)
			break
		}
		if len(a) > 1 && a[0] == '-' && !numeric.MatchString(a) {
			flags = append(flags, a)
			name := strings.TrimLeft(a, "-")
			if strings.Contains(name, "=") {
				continue
			}
			if f := fs.Lookup(name); f != nil && !isBoolFlag(f) && i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
			continue
		}
		pos = append(pos, a)
	}
	if err := fs.Parse(flags); err != nil {
		return nil, output.Usage("%v", err)
	}
	return pos, nil
}

func isBoolFlag(f *flag.Flag) bool {
	b, ok := f.Value.(interface{ IsBoolFlag() bool })
	return ok && b.IsBoolFlag()
}

// needArgs checks the number of positionals and returns a usage error that
// names the command's synopsis.
func needArgs(c string, usage string, pos []string, min, max int) error {
	if len(pos) < min || (max >= 0 && len(pos) > max) {
		return output.Usage("usage: spotify-helper %s %s", c, usage)
	}
	return nil
}

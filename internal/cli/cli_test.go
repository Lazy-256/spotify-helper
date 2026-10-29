package cli

import (
	"bytes"
	"encoding/json"
	"flag"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
)

// testEnv returns an Env writing to buffers, with an isolated XDG tree.
func testEnv(t *testing.T) (*Env, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	var out, errb bytes.Buffer
	dir := t.TempDir()
	vars := map[string]string{
		"XDG_CONFIG_HOME": dir + "/config",
		"XDG_CACHE_HOME":  dir + "/cache",
		"HOME":            dir + "/home",
	}
	return &Env{
		Stdout:       &out,
		Stderr:       &errb,
		Getenv:       func(k string) string { return vars[k] },
		Now:          func() time.Time { return time.Unix(1_700_000_000, 0) },
		APIBase:      defaultAPIBase,
		AccountsBase: defaultAccountsBase,
		CoverHTTP:    &http.Client{Timeout: 5 * time.Second},
		OpenBrowser:  func(string) error { return nil },
		SettleDelay:  time.Nanosecond,
	}, &out, &errb
}

func errorCode(t *testing.T, stderr *bytes.Buffer) string {
	t.Helper()
	var doc struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	// The error object is the last line of stderr (login may print the
	// authorization URL before it).
	lines := strings.Split(strings.TrimRight(stderr.String(), "\n"), "\n")
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &doc); err != nil {
		t.Fatalf("last stderr line is not a JSON error: %q (%v)", stderr.String(), err)
	}
	return doc.Error.Code
}

func TestVersion(t *testing.T) {
	env, out, errb := testEnv(t)
	if code := Run([]string{"version"}, env); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errb.String())
	}
	var doc struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil || doc.Version == "" {
		t.Fatalf("bad version output %q (%v)", out.String(), err)
	}
	if errb.Len() != 0 {
		t.Errorf("stderr should be empty, got %q", errb.String())
	}
}

func TestHelpGoesToStdoutWithExitZero(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"-h"}, {"version", "--help"}} {
		env, out, errb := testEnv(t)
		if code := Run(args, env); code != 0 {
			t.Fatalf("%v: exit %d", args, code)
		}
		if !strings.Contains(out.String(), "spotify-helper") || errb.Len() != 0 {
			t.Errorf("%v: stdout %q stderr %q", args, out.String(), errb.String())
		}
	}
}

func TestUnknownSubcommand(t *testing.T) {
	env, out, errb := testEnv(t)
	if code := Run([]string{"frobnicate"}, env); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	if out.Len() != 0 {
		t.Errorf("stdout must be empty on failure, got %q", out.String())
	}
	if got := errorCode(t, errb); got != "usage" {
		t.Errorf("code %q, want usage", got)
	}
}

func TestMissingCommand(t *testing.T) {
	env, _, errb := testEnv(t)
	if code := Run(nil, env); code != 2 || errorCode(t, errb) != "usage" {
		t.Errorf("exit %d stderr %q", code, errb.String())
	}
}

func TestVersionRejectsExtraArgs(t *testing.T) {
	env, _, errb := testEnv(t)
	if code := Run([]string{"version", "x"}, env); code != 2 || errorCode(t, errb) != "usage" {
		t.Errorf("exit %d stderr %q", code, errb.String())
	}
}

func TestParseArgsInterleaved(t *testing.T) {
	newFS := func() (*flag.FlagSet, *string, *bool) {
		fs := flag.NewFlagSet("t", flag.ContinueOnError)
		device := fs.String("device", "", "")
		play := fs.Bool("play", false, "")
		return fs, device, play
	}
	tests := []struct {
		name   string
		args   []string
		want   []string
		device string
		play   bool
	}{
		{"flags after positionals", []string{"daft", "punk", "--device", "D1"}, []string{"daft", "punk"}, "D1", false},
		{"bool flag does not eat next arg", []string{"--play", "X"}, []string{"X"}, "", true},
		{"equals form", []string{"X", "--device=D2"}, []string{"X"}, "D2", false},
		{"negative number is positional", []string{"-10"}, []string{"-10"}, "", false},
		{"signed number with flag", []string{"-5000", "--device", "D"}, []string{"-5000"}, "D", false},
		{"double dash ends flags", []string{"--", "--device"}, []string{"--device"}, "", false},
	}
	for _, tt := range tests {
		fs, device, play := newFS()
		got, err := parseArgs(fs, tt.args)
		if err != nil {
			t.Errorf("%s: %v", tt.name, err)
			continue
		}
		if !reflect.DeepEqual(got, tt.want) || *device != tt.device || *play != tt.play {
			t.Errorf("%s: pos %q device %q play %v", tt.name, got, *device, *play)
		}
	}
}

func TestParseArgsUnknownFlagIsUsage(t *testing.T) {
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	_, err := parseArgs(fs, []string{"--nope"})
	if err == nil || !strings.Contains(err.Error(), "usage") {
		t.Errorf("want usage error, got %v", err)
	}
}

func TestNeedArgs(t *testing.T) {
	if err := needArgs("volume", "<value>", nil, 1, 1); err == nil || !strings.Contains(err.Error(), "usage: spotify-helper volume <value>") {
		t.Errorf("missing argument should be a usage error, got %v", err)
	}
	if err := needArgs("volume", "<value>", []string{"1", "2"}, 1, 1); err == nil {
		t.Error("extra argument should be a usage error")
	}
	if err := needArgs("search", "<query>", []string{"a", "b", "c"}, 1, -1); err != nil {
		t.Errorf("unbounded max should accept any count, got %v", err)
	}
}

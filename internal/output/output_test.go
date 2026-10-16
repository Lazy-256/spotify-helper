package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestExitCodes(t *testing.T) {
	tests := []struct {
		code Code
		want int
	}{
		{CodeInternal, 1},
		{CodeAPIError, 1},
		{CodeUsage, 2},
		{CodeNotConfigured, 3},
		{CodeNotAuthenticated, 3},
		{CodeScopesMissing, 3},
		{CodeNoActiveDevice, 4},
		{CodeDeviceNotFound, 4},
		{CodeAmbiguousDevice, 4},
		{CodeRateLimited, 5},
		{CodePremiumRequired, 6},
		{CodeNetworkError, 7},
		{Code("something_new"), 1},
	}
	for _, tt := range tests {
		if got := tt.code.ExitCode(); got != tt.want {
			t.Errorf("%s: exit code %d, want %d", tt.code, got, tt.want)
		}
	}
}

func TestWriteErrorShape(t *testing.T) {
	var buf bytes.Buffer
	WriteError(&buf, Errorf(CodeNoActiveDevice, "no device"))
	if !strings.HasSuffix(buf.String(), "\n") || strings.Count(buf.String(), "\n") != 1 {
		t.Fatalf("want exactly one line, got %q", buf.String())
	}
	var doc map[string]map[string]any
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	e := doc["error"]
	if e["code"] != "no_active_device" || e["message"] != "no device" {
		t.Errorf("unexpected error body: %v", e)
	}
	if _, ok := e["retry_after"]; ok {
		t.Errorf("retry_after must be absent for non rate-limit errors: %v", e)
	}
}

func TestRateLimitedCarriesRetryAfter(t *testing.T) {
	var buf bytes.Buffer
	WriteError(&buf, RateLimited(12))
	var doc struct {
		Error struct {
			Code       string `json:"code"`
			RetryAfter int    `json:"retry_after"`
		} `json:"error"`
	}
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Error.Code != "rate_limited" || doc.Error.RetryAfter != 12 {
		t.Errorf("got %+v", doc.Error)
	}
	if got := RateLimited(0).RetryAfter; got != 1 {
		t.Errorf("RetryAfter floor: got %d, want 1", got)
	}
}

func TestWrap(t *testing.T) {
	inner := Usage("bad")
	if got := Wrap(fmt.Errorf("ctx: %w", inner)); got != inner {
		t.Errorf("Wrap should unwrap *Error, got %#v", got)
	}
	got := Wrap(errors.New("boom"))
	if got.Code != CodeInternal || got.Message != "boom" {
		t.Errorf("unknown error should be internal, got %#v", got)
	}
}

func TestWriteOKAndNoHTMLEscape(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteOK(&buf); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "{\"ok\":true}\n" {
		t.Errorf("got %q", buf.String())
	}
	buf.Reset()
	_ = WriteJSON(&buf, map[string]string{"n": "A&B <x>"})
	if buf.String() != "{\"n\":\"A&B <x>\"}\n" {
		t.Errorf("HTML must not be escaped, got %q", buf.String())
	}
}

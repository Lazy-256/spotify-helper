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
		{CodeTracksUnavailable, 1},
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

func TestErrorAddedAndTotal(t *testing.T) {
	var buf bytes.Buffer
	e := RateLimited(12)
	e.Added, e.Total = 8, 30
	WriteError(&buf, e)
	var doc struct {
		Error map[string]any `json:"error"`
	}
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Error["added"] != float64(8) || doc.Error["total"] != float64(30) || doc.Error["retry_after"] != float64(12) {
		t.Errorf("got %v", doc.Error)
	}

	buf.Reset()
	doc.Error = nil
	WriteError(&buf, Errorf(CodeTracksUnavailable, "no items"))
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Error["code"] != "tracks_unavailable" {
		t.Errorf("got %v", doc.Error)
	}
	for _, k := range []string{"added", "total", "retry_after"} {
		if _, ok := doc.Error[k]; ok {
			t.Errorf("%s must be absent unless set: %v", k, doc.Error)
		}
	}
}

func TestQueueAddAllResultShape(t *testing.T) {
	b, err := Encode(QueueAddAllResult{OK: true, URI: "spotify:album:a", Added: 3, Skipped: 1, Total: 9, Offset: 2, Truncated: true})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"ok":true,"uri":"spotify:album:a","added":3,"skipped":1,"total":9,"offset":2,"truncated":true}` + "\n"
	if string(b) != want {
		t.Errorf("got %s, want %s", b, want)
	}
	b, _ = Encode(QueueAddAllResult{OK: true})
	for _, k := range []string{`"added":0`, `"skipped":0`, `"total":0`, `"offset":0`, `"truncated":false`} {
		if !strings.Contains(string(b), k) {
			t.Errorf("zero values must be written: %s lacks %s", b, k)
		}
	}
}

func TestErrorSkipped(t *testing.T) {
	var buf bytes.Buffer
	e := RateLimited(12)
	e.Skipped = 30
	WriteError(&buf, e)
	var doc struct {
		Error map[string]any `json:"error"`
	}
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Error["skipped"] != float64(30) || doc.Error["retry_after"] != float64(12) {
		t.Errorf("got %v", doc.Error)
	}

	buf.Reset()
	doc.Error = nil
	WriteError(&buf, RateLimited(5))
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if _, ok := doc.Error["skipped"]; ok {
		t.Errorf("skipped must be absent unless set: %v", doc.Error)
	}
}

func TestQueueClearResultShape(t *testing.T) {
	b, err := Encode(QueueClearResult{OK: true, Skipped: 37, Empty: true})
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"ok":true,"skipped":37,"empty":true}` + "\n"; string(b) != want {
		t.Errorf("got %s, want %s", b, want)
	}
	b, _ = Encode(QueueClearResult{OK: true})
	for _, k := range []string{`"skipped":0`, `"empty":false`} {
		if !strings.Contains(string(b), k) {
			t.Errorf("zero values must be written: %s lacks %s", b, k)
		}
	}
}

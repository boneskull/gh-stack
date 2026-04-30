package style

import (
	"strings"
	"testing"
)

func TestHyperlinkColorsDisabled(t *testing.T) {
	s := NewWithColor(false)
	got := s.Hyperlink("PR #42", "https://github.com/owner/repo/pull/42")
	want := "PR #42 (https://github.com/owner/repo/pull/42)"
	if got != want {
		t.Errorf("Hyperlink fallback: got %q, want %q", got, want)
	}
}

func TestHyperlinkColorsEnabled(t *testing.T) {
	s := NewWithColor(true)
	got := s.Hyperlink("PR #42", "https://github.com/owner/repo/pull/42")
	// OSC 8 sequence wraps the visible text
	if !strings.Contains(got, "PR #42") {
		t.Errorf("Hyperlink should contain visible text; got %q", got)
	}
	if !strings.Contains(got, "https://github.com/owner/repo/pull/42") {
		t.Errorf("Hyperlink should contain URL; got %q", got)
	}
	if !strings.HasPrefix(got, "\x1b]8;;") {
		t.Errorf("Hyperlink should start with OSC 8 sequence; got %q", got)
	}
}

func TestHyperlinkEmptyURL(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		s := NewWithColor(enabled)
		got := s.Hyperlink("PR #42", "")
		if got != "PR #42" {
			t.Errorf("Hyperlink with empty URL (enabled=%v): got %q, want %q", enabled, got, "PR #42")
		}
	}
}

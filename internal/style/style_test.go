package style

import "testing"

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
	want := "\x1b]8;;https://github.com/owner/repo/pull/42\x1b\\PR #42\x1b]8;;\x1b\\"
	if got != want {
		t.Errorf("Hyperlink OSC 8: got %q, want %q", got, want)
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

package diff

import (
	"regexp"
	"testing"
)

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return ansiRe.ReplaceAllString(s, "")
}

func TestDiffIdentical(t *testing.T) {
	// The underlying diff library only emits chunks around differences, so two
	// fully identical inputs produce no chunks (and thus no output) at all.
	got := stripANSI(Diff("a\nb\nc", "a\nb\nc"))
	if got != "" {
		t.Errorf("got %q, want empty diff for identical input", got)
	}
}

func TestDiffBothEmpty(t *testing.T) {
	got := stripANSI(Diff("", ""))
	if got != "" {
		t.Errorf("expected empty diff, got %q", got)
	}
}

func TestDiffChangedLine(t *testing.T) {
	got := stripANSI(Diff("a\nb\nc", "a\nB\nc"))
	want := " a\n-b\n+B\n c"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDiffAddedLine(t *testing.T) {
	got := stripANSI(Diff("a\nb\nc", "a\nb\nc\nd"))
	want := " a\n b\n c\n+d"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDiffRemovedLine(t *testing.T) {
	got := stripANSI(Diff("a\nb\nc\nd", "a\nb\nc"))
	want := " a\n b\n c\n-d"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDiffCompletelyDifferent(t *testing.T) {
	got := stripANSI(Diff("a", "b"))
	want := "-a\n+b"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDiffNoTrailingNewline(t *testing.T) {
	got := Diff("a\nb\nc", "a\nb\nc\nd")
	if len(got) > 0 && got[len(got)-1] == '\n' {
		t.Errorf("expected no trailing newline, got %q", got)
	}
}

func TestDiffOneSideEmpty(t *testing.T) {
	got := stripANSI(Diff("", "a\nb"))
	want := "-\n+a\n+b"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

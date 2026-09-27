package logger

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
)

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return ansiRe.ReplaceAllString(s, "")
}

func TestNew(t *testing.T) {
	out := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	l := New(out, errBuf)

	if l.out != out {
		t.Errorf("out not stored correctly")
	}
	if l.err != errBuf {
		t.Errorf("err not stored correctly")
	}
	if l.pid != os.Getpid() {
		t.Errorf("pid = %d, want %d", l.pid, os.Getpid())
	}
}

func TestInfoWritesToOut(t *testing.T) {
	out := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	l := New(out, errBuf)

	l.Info("hello %s", "world")

	got := stripANSI(out.String())
	want := fmt.Sprintf("[%d] hello world\n", os.Getpid())
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if errBuf.Len() != 0 {
		t.Errorf("expected err writer untouched, got %q", errBuf.String())
	}
}

func TestWarnWritesToOut(t *testing.T) {
	out := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	l := New(out, errBuf)

	l.Warn("careful: %d", 42)

	got := stripANSI(out.String())
	want := fmt.Sprintf("[%d] careful: 42\n", os.Getpid())
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if errBuf.Len() != 0 {
		t.Errorf("expected err writer untouched, got %q", errBuf.String())
	}
}

func TestFatalWritesToErr(t *testing.T) {
	out := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	l := New(out, errBuf)

	l.Fatal("boom: %v", "oops")

	got := stripANSI(errBuf.String())
	want := fmt.Sprintf("[%d] boom: oops\n", os.Getpid())
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if out.Len() != 0 {
		t.Errorf("expected out writer untouched, got %q", out.String())
	}
}

func TestNoFormatArgs(t *testing.T) {
	out := new(bytes.Buffer)
	l := New(out, new(bytes.Buffer))

	l.Info("plain message")

	got := stripANSI(out.String())
	if !strings.Contains(got, "plain message") {
		t.Errorf("expected message in output, got %q", got)
	}
}

func TestMultipleCallsAppend(t *testing.T) {
	out := new(bytes.Buffer)
	l := New(out, new(bytes.Buffer))

	l.Info("first")
	l.Info("second")

	got := stripANSI(out.String())
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %q", len(lines), got)
	}
	if !strings.Contains(lines[0], "first") || !strings.Contains(lines[1], "second") {
		t.Errorf("unexpected line contents: %q", got)
	}
}

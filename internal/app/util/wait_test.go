package util

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kraken-hub/kraken/internal/runtime"
)

// muxFrame builds a single Docker log-multiplexed frame (stdout stream) for payload.
func muxFrame(payload string) []byte {
	hdr := make([]byte, 8)
	hdr[0] = 1 // stdout
	binary.BigEndian.PutUint32(hdr[4:8], uint32(len(payload)))
	return append(hdr, []byte(payload)...)
}

func TestReadMuxedLines(t *testing.T) {
	t.Run("single line with trailing newline", func(t *testing.T) {
		r := bytes.NewReader(muxFrame("hello\n"))
		ch := make(chan string, 8)
		readMuxedLines(r, ch)
		var got []string
		for l := range ch {
			got = append(got, l)
		}
		want := []string{"hello"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %#v, want %#v", got, want)
		}
	})

	t.Run("multiple lines in one frame", func(t *testing.T) {
		r := bytes.NewReader(muxFrame("line1\nline2\nline3\n"))
		ch := make(chan string, 8)
		readMuxedLines(r, ch)
		var got []string
		for l := range ch {
			got = append(got, l)
		}
		want := []string{"line1", "line2", "line3"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %#v, want %#v", got, want)
		}
	})

	t.Run("frame without trailing newline sends final partial line", func(t *testing.T) {
		r := bytes.NewReader(muxFrame("nopartialnewline"))
		ch := make(chan string, 8)
		readMuxedLines(r, ch)
		var got []string
		for l := range ch {
			got = append(got, l)
		}
		want := []string{"nopartialnewline"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %#v, want %#v", got, want)
		}
	})

	t.Run("blank lines within a frame are preserved", func(t *testing.T) {
		r := bytes.NewReader(muxFrame("a\n\nb\n"))
		ch := make(chan string, 8)
		readMuxedLines(r, ch)
		var got []string
		for l := range ch {
			got = append(got, l)
		}
		want := []string{"a", "", "b"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %#v, want %#v", got, want)
		}
	})

	t.Run("zero-length frame skipped", func(t *testing.T) {
		hdr := make([]byte, 8)
		binary.BigEndian.PutUint32(hdr[4:8], 0)
		buf := append(hdr, muxFrame("after\n")...)
		r := bytes.NewReader(buf)
		ch := make(chan string, 8)
		readMuxedLines(r, ch)
		var got []string
		for l := range ch {
			got = append(got, l)
		}
		want := []string{"after"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %#v, want %#v", got, want)
		}
	})

	t.Run("carriage returns stripped", func(t *testing.T) {
		r := bytes.NewReader(muxFrame("windows-line\r\n"))
		ch := make(chan string, 8)
		readMuxedLines(r, ch)
		var got []string
		for l := range ch {
			got = append(got, l)
		}
		want := []string{"windows-line"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %#v, want %#v", got, want)
		}
	})

	t.Run("multiple frames across reads", func(t *testing.T) {
		var buf bytes.Buffer
		buf.Write(muxFrame("first\n"))
		buf.Write(muxFrame("second\n"))
		ch := make(chan string, 8)
		readMuxedLines(&buf, ch)
		var got []string
		for l := range ch {
			got = append(got, l)
		}
		want := []string{"first", "second"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %#v, want %#v", got, want)
		}
	})

	t.Run("truncated header closes channel without panic", func(t *testing.T) {
		r := bytes.NewReader([]byte{1, 0, 0})
		ch := make(chan string, 8)
		readMuxedLines(r, ch)
		_, ok := <-ch
		if ok {
			t.Error("expected channel to be closed with no values")
		}
	})
}

type closerReader struct {
	io.Reader
	closed bool
}

func (c *closerReader) Close() error {
	c.closed = true
	return nil
}

func TestWaitWithDirective(t *testing.T) {
	t.Run("falls back to plain timer when logs unavailable", func(t *testing.T) {
		rt := &fakeRuntime{
			ContainerLogsFn: func(ctx context.Context, id string, opts runtime.LogsOptions) (io.ReadCloser, error) {
				return nil, errors.New("logs unavailable")
			},
		}
		tk := newTestKraken(t, rt)
		start := time.Now()
		err := WaitWithDirective(tk.Kraken, "cid", "web.myapp", 20*time.Millisecond)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if elapsed := time.Since(start); elapsed < 20*time.Millisecond {
			t.Errorf("returned too quickly: %v", elapsed)
		}
	})

	t.Run("returns when defaultWait expires without directive", func(t *testing.T) {
		body := muxFrame("just a log line\n")
		rt := &fakeRuntime{
			ContainerLogsFn: func(ctx context.Context, id string, opts runtime.LogsOptions) (io.ReadCloser, error) {
				return &closerReader{Reader: bytes.NewReader(body)}, nil
			},
		}
		tk := newTestKraken(t, rt)
		err := WaitWithDirective(tk.Kraken, "cid", "web.myapp", 30*time.Millisecond)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := tk.stdout()
		if !strings.Contains(out, "just a log line") {
			t.Errorf("stdout = %q, want it to contain the log line", out)
		}
	})

	t.Run("directive cuts wait short and continues streaming", func(t *testing.T) {
		body := muxFrame("before\n@kraken wait 10ms\nafter\n")
		rt := &fakeRuntime{
			ContainerLogsFn: func(ctx context.Context, id string, opts runtime.LogsOptions) (io.ReadCloser, error) {
				return &closerReader{Reader: bytes.NewReader(body)}, nil
			},
		}
		tk := newTestKraken(t, rt)
		start := time.Now()
		err := WaitWithDirective(tk.Kraken, "cid", "web.myapp", 5*time.Second)
		elapsed := time.Since(start)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if elapsed >= 5*time.Second {
			t.Errorf("directive did not cut the wait short: elapsed %v", elapsed)
		}
		out := tk.stdout()
		if !strings.Contains(out, "before") {
			t.Errorf("stdout missing pre-directive line: %q", out)
		}
		if !strings.Contains(out, "after") {
			t.Errorf("stdout missing post-directive line: %q", out)
		}
		if strings.Contains(out, "@kraken wait") {
			t.Errorf("directive line should not be printed: %q", out)
		}
	})

	t.Run("leading blank lines suppressed but mid-stream blank lines kept", func(t *testing.T) {
		body := muxFrame("\n\nfirst\n\nsecond\n")
		rt := &fakeRuntime{
			ContainerLogsFn: func(ctx context.Context, id string, opts runtime.LogsOptions) (io.ReadCloser, error) {
				return &closerReader{Reader: bytes.NewReader(body)}, nil
			},
		}
		tk := newTestKraken(t, rt)
		if err := WaitWithDirective(tk.Kraken, "cid", "web.myapp", 30*time.Millisecond); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := tk.stdout()
		// Leading blank lines before "first" must be suppressed, i.e. output should
		// not start with a run of blank lines followed directly by more blanks before content.
		if len(out) == 0 {
			t.Fatal("expected some output")
		}
		if out[0] == '\n' && out[1] == '\n' {
			t.Errorf("leading blank lines were not suppressed: %q", out)
		}
		if !strings.Contains(out, "first\n\nsecond") {
			t.Errorf("expected mid-stream blank line between first and second preserved: %q", out)
		}
	})

	t.Run("no directive found leaves stream open until defaultWait via long-lived pipe", func(t *testing.T) {
		r, w := io.Pipe()
		t.Cleanup(func() { w.Close() })
		rt := &fakeRuntime{
			ContainerLogsFn: func(ctx context.Context, id string, opts runtime.LogsOptions) (io.ReadCloser, error) {
				return io.NopCloser(r), nil
			},
		}
		tk := newTestKraken(t, rt)
		start := time.Now()
		err := WaitWithDirective(tk.Kraken, "cid", "web.myapp", 15*time.Millisecond)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if elapsed := time.Since(start); elapsed < 15*time.Millisecond {
			t.Errorf("returned before defaultWait elapsed: %v", elapsed)
		}
	})
}

package docker

import (
	"bytes"
	"context"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

// GetTtySize relies on moby/term recognizing *os.File; anything else (or a
// non-tty *os.File such as a regular file) is treated as "not a tty" and
// yields (0, 0).

func TestGetTtySize_NonFileInput(t *testing.T) {
	h, w := GetTtySize(&bytes.Buffer{})
	if h != 0 || w != 0 {
		t.Errorf("GetTtySize(bytes.Buffer) = (%d, %d), want (0, 0)", h, w)
	}
}

func TestGetTtySize_NilInput(t *testing.T) {
	h, w := GetTtySize(nil)
	if h != 0 || w != 0 {
		t.Errorf("GetTtySize(nil) = (%d, %d), want (0, 0)", h, w)
	}
}

func TestGetTtySize_RegularFile(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "tty-test")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer f.Close()

	h, w := GetTtySize(f)
	if h != 0 || w != 0 {
		t.Errorf("GetTtySize(regular file) = (%d, %d), want (0, 0)", h, w)
	}
}

func TestResizeTtyTo_ZeroSizeIsNoop(t *testing.T) {
	// height == 0 && width == 0 must return nil without touching the client,
	// so a nil client is safe to pass here.
	err := resizeTtyTo(context.Background(), nil, "container-id", 0, 0, false)
	if err != nil {
		t.Errorf("resizeTtyTo with zero size = %v, want nil", err)
	}
}

type resizeCall struct {
	id     string
	height uint
	width  uint
	isExec bool
}

type fakeContainerAPIClient struct {
	client.ContainerAPIClient
	resizeCalls     []resizeCall
	execResizeCalls []resizeCall
	resizeErr       error
	execResizeErr   error
}

func (f *fakeContainerAPIClient) ContainerResize(ctx context.Context, container string, options container.ResizeOptions) error {
	f.resizeCalls = append(f.resizeCalls, resizeCall{id: container, height: options.Height, width: options.Width})
	return f.resizeErr
}

func (f *fakeContainerAPIClient) ContainerExecResize(ctx context.Context, execID string, options container.ResizeOptions) error {
	f.execResizeCalls = append(f.execResizeCalls, resizeCall{id: execID, height: options.Height, width: options.Width, isExec: true})
	return f.execResizeErr
}

func TestResizeTtyTo_NonZeroCallsContainerResize(t *testing.T) {
	fc := &fakeContainerAPIClient{}
	err := resizeTtyTo(context.Background(), fc, "abc123", 24, 80, false)
	if err != nil {
		t.Fatalf("resizeTtyTo: %v", err)
	}
	if len(fc.resizeCalls) != 1 {
		t.Fatalf("expected 1 ContainerResize call, got %d", len(fc.resizeCalls))
	}
	if fc.resizeCalls[0].id != "abc123" || fc.resizeCalls[0].height != 24 || fc.resizeCalls[0].width != 80 {
		t.Errorf("unexpected call: %+v", fc.resizeCalls[0])
	}
	if len(fc.execResizeCalls) != 0 {
		t.Errorf("expected 0 ContainerExecResize calls, got %d", len(fc.execResizeCalls))
	}
}

func TestResizeTtyTo_ExecUsesContainerExecResize(t *testing.T) {
	fc := &fakeContainerAPIClient{}
	err := resizeTtyTo(context.Background(), fc, "exec123", 10, 20, true)
	if err != nil {
		t.Fatalf("resizeTtyTo: %v", err)
	}
	if len(fc.execResizeCalls) != 1 {
		t.Fatalf("expected 1 ContainerExecResize call, got %d", len(fc.execResizeCalls))
	}
	if len(fc.resizeCalls) != 0 {
		t.Errorf("expected 0 ContainerResize calls, got %d", len(fc.resizeCalls))
	}
}

func TestResizeTtyTo_PropagatesError(t *testing.T) {
	wantErr := os.ErrClosed
	fc := &fakeContainerAPIClient{resizeErr: wantErr}
	err := resizeTtyTo(context.Background(), fc, "abc", 1, 1, false)
	if err != wantErr {
		t.Errorf("resizeTtyTo error = %v, want %v", err, wantErr)
	}
}

func TestResizeTty_NonTtyStdIsNoop(t *testing.T) {
	fc := &fakeContainerAPIClient{}
	// GetTtySize(&bytes.Buffer{}) returns (0,0), so resizeTty should not
	// invoke the client at all.
	err := resizeTty(context.Background(), fc, &bytes.Buffer{}, "abc", false)
	if err != nil {
		t.Fatalf("resizeTty: %v", err)
	}
	if len(fc.resizeCalls) != 0 || len(fc.execResizeCalls) != 0 {
		t.Errorf("expected no client calls for a non-tty std, got resize=%v execResize=%v", fc.resizeCalls, fc.execResizeCalls)
	}
}

func TestInitTtySize_SuccessDoesNotRetry(t *testing.T) {
	var calls int32
	resizeFn := func(ctx context.Context, c client.ContainerAPIClient, std interface{}, id string, isExec bool) error {
		atomic.AddInt32(&calls, 1)
		return nil
	}

	initTtySize(context.Background(), nil, nil, "id", false, resizeFn)

	// Give any (unexpected) background goroutine a chance to run.
	time.Sleep(20 * time.Millisecond)

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("resizeFn called %d times, want 1 (no retries on success)", got)
	}
}

func TestInitTtySize_RetriesOnFailureThenSucceeds(t *testing.T) {
	var calls int32
	done := make(chan struct{})
	resizeFn := func(ctx context.Context, c client.ContainerAPIClient, std interface{}, id string, isExec bool) error {
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			return os.ErrClosed
		}
		close(done)
		return nil
	}

	initTtySize(context.Background(), nil, nil, "id", false, resizeFn)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for retries to succeed")
	}

	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Errorf("resizeFn called %d times, want 3", got)
	}
}

func TestInitTtySize_AllRetriesFail(t *testing.T) {
	var calls int32
	resizeFn := func(ctx context.Context, c client.ContainerAPIClient, std interface{}, id string, isExec bool) error {
		atomic.AddInt32(&calls, 1)
		return os.ErrClosed
	}

	initTtySize(context.Background(), nil, nil, "id", false, resizeFn)

	// 1 synchronous call + up to 10 retries in the background goroutine.
	deadline := time.After(3 * time.Second)
	for {
		if atomic.LoadInt32(&calls) == 11 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("resizeFn called %d times, want 11 (1 initial + 10 retries)", atomic.LoadInt32(&calls))
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestInitTtySize_NilResizeFnDefaultsToResizeTty(t *testing.T) {
	// With a nil resizeTtyFunc, initTtySize falls back to resizeTty, which
	// short-circuits to a no-op for a non-tty std and never touches the
	// (nil) client.
	done := make(chan struct{})
	go func() {
		initTtySize(context.Background(), nil, &bytes.Buffer{}, "id", false, nil)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("initTtySize with nil resizeTtyFunc did not return in time")
	}
}

func TestMonitorTtySize_NonTtyStdDoesNotPanic(t *testing.T) {
	err := MonitorTtySize(context.Background(), nil, &bytes.Buffer{}, "id", false)
	if err != nil {
		t.Errorf("MonitorTtySize = %v, want nil", err)
	}
}

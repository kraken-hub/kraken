package app

import (
	"context"
	"errors"
	"syscall"
	"testing"
	"time"
)

func TestContextWithSigterm(t *testing.T) {
	t.Run("signal cancels context with SignalError cause", func(t *testing.T) {
		ctx := contextWithSigterm(context.Background())
		if err := syscall.Kill(syscall.Getpid(), syscall.SIGINT); err != nil {
			t.Fatal(err)
		}
		select {
		case <-ctx.Done():
		case <-time.After(5 * time.Second):
			t.Fatal("context not cancelled after SIGINT")
		}
		sigErr := InterruptedBy(ctx)
		if sigErr == nil {
			t.Fatalf("InterruptedBy = nil, cause = %v", context.Cause(ctx))
		}
		if sigErr.Signal != syscall.SIGINT {
			t.Errorf("Signal = %v, want SIGINT", sigErr.Signal)
		}
		if got := sigErr.ExitCode(); got != 130 {
			t.Errorf("ExitCode = %d, want 130", got)
		}
	})

	t.Run("plain cancellation is not reported as a signal", func(t *testing.T) {
		ctx, cancel := context.WithCancelCause(context.Background())
		cancel(errors.New("other"))
		if sigErr := InterruptedBy(ctx); sigErr != nil {
			t.Errorf("InterruptedBy = %v, want nil", sigErr)
		}
		if sigErr := InterruptedBy(context.Background()); sigErr != nil {
			t.Errorf("InterruptedBy(background) = %v, want nil", sigErr)
		}
	})
}

package app

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"
)

// SignalError is the context cancellation cause set when kraken receives
// SIGINT or SIGTERM.
type SignalError struct {
	Signal syscall.Signal
}

func (e *SignalError) Error() string { return e.Signal.String() + " signal received" }

// ExitCode returns the conventional shell exit code for a signal: 128 + signal number.
func (e *SignalError) ExitCode() int { return 128 + int(e.Signal) }

// InterruptedBy returns the signal that cancelled ctx, or nil if ctx was not
// cancelled by a signal.
func InterruptedBy(ctx context.Context) *SignalError {
	var sigErr *SignalError
	if errors.As(context.Cause(ctx), &sigErr) {
		return sigErr
	}
	return nil
}

func contextWithSigterm(ctx context.Context) context.Context {
	ctx, cancel := context.WithCancelCause(ctx)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		// Restore default handling so a second signal terminates immediately.
		signal.Stop(sigCh)
		cancel(&SignalError{Signal: sig.(syscall.Signal)})
	}()
	return ctx
}

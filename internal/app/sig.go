package app

import (
	"context"
	"os/signal"
	"syscall"
)

func contextWithSigterm(ctx context.Context) context.Context {
	ctx, cancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-ctx.Done()
		cancel()
	}()
	return ctx
}

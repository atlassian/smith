package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/ash2k/smith/pkg/app"
)

func main() {
	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()
	cancelOnInterrupt(ctx, cancelFunc)

	a := app.App{}

	if err := a.Run(ctx); err != nil && err != context.Canceled && err != context.DeadlineExceeded {
		log.Fatalln(err)
	}
}

// cancelOnInterrupt calls f when os.Interrupt or SIGTERM is received.
// It ignores subsequent interrupts on purpose - program should exit correctly after the first signal.
func cancelOnInterrupt(ctx context.Context, f context.CancelFunc) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		select {
		case <-ctx.Done():
		case <-c:
			f()
		}
	}()
}

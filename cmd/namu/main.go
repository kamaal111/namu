package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/kamaal111/namu/internal/cli"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	dir, err := os.Getwd()
	if err == nil {
		err = cli.New(dir).ExecuteContext(ctx)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "namu:", err)
		if errors.Is(err, context.Canceled) {
			os.Exit(130)
		}
		os.Exit(1)
	}
}

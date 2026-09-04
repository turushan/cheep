package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/turushan/cheep/internal/buildinfo"
	"github.com/turushan/cheep/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(cli.Execute(os.Args[1:], cli.Options{
		Context: ctx,
		Stdin:   os.Stdin,
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
		Build:   buildinfo.Current(),
	}))
}

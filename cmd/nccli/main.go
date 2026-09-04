package main

import (
	"os"

	"github.com/turushan/nccli/internal/buildinfo"
	"github.com/turushan/nccli/internal/cli"
)

func main() {
	os.Exit(cli.Execute(os.Args[1:], cli.Options{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Build:  buildinfo.Current(),
	}))
}

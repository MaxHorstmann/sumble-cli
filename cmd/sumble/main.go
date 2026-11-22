// Command sumble is a lightweight CLI for the Sumble API.
package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/MaxHorstmann/sumble-cli/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := cli.Main(ctx, os.Args[1:], os.Getenv, os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

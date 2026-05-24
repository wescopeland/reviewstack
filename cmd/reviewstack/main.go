package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/wescopeland/reviewstack/internal/app"
	"github.com/wescopeland/reviewstack/internal/cli"
	"github.com/wescopeland/reviewstack/internal/doctor"
	"github.com/wescopeland/reviewstack/internal/launcher"
)

func main() {
	opts, err := cli.ParseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "reviewstack: %v\n", err)
		os.Exit(2)
	}

	if opts.Doctor {
		os.Exit(doctor.Run(os.Stdout))
	}

	if opts.NeedsLauncher() {
		if opts.NoTUI {
			fmt.Fprintln(os.Stderr, "reviewstack: specify --pr, --uncommitted, --rereview --pr, or omit --no-tui for the mode picker")
			os.Exit(2)
		}
		result, err := launcher.Run(opts.Base)
		if err != nil {
			fmt.Fprintf(os.Stderr, "reviewstack: %v\n", err)
			os.Exit(1)
		}
		if result.Cancelled {
			return
		}
		launcher.ApplyResult(opts, result)
	}

	if err := opts.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "reviewstack: %v\n", err)
		os.Exit(2)
	}

	if !opts.HasReviewTarget() {
		fmt.Fprintln(os.Stderr, "reviewstack: no review target selected")
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := app.New(opts).Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "reviewstack: %v\n", err)
		os.Exit(1)
	}
}

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/tralljf/stress-test-go-expert/internal/config"
	"github.com/tralljf/stress-test-go-expert/internal/loadtest"
)

func main() {
	cfg, err := config.Parse(os.Args[1:], os.Stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Printf("Running %d requests against %s with concurrency %d (timeout %s, max redirects %d)...\n",
		cfg.Requests, cfg.URL, cfg.Concurrency, cfg.Timeout, cfg.MaxRedirects)

	client := loadtest.NewHTTPClient(cfg.Concurrency, cfg.Timeout, cfg.MaxRedirects)
	report := loadtest.Run(ctx, client, loadtest.Options{
		URL:         cfg.URL,
		Requests:    cfg.Requests,
		Concurrency: cfg.Concurrency,
	})
	report.Print(os.Stdout)
}

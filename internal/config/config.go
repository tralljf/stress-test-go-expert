// Package config parses and validates the command line flags of the CLI.
package config

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"time"
)

// DefaultTimeout is the per-request timeout used when --timeout is omitted.
const DefaultTimeout = 10 * time.Second

// DefaultMaxRedirects matches the default limit of Go's http.Client.
const DefaultMaxRedirects = 10

// Config holds the validated parameters of a load test run.
type Config struct {
	URL          string
	Requests     int
	Concurrency  int
	Timeout      time.Duration
	MaxRedirects int
}

// Parse reads the flags from args (without the program name) and validates them.
// Both "--flag=value" and "--flag value" forms are accepted.
// When concurrency is greater than requests it is capped to requests.
func Parse(args []string, output io.Writer) (Config, error) {
	fs := flag.NewFlagSet("stress-test", flag.ContinueOnError)
	fs.SetOutput(output)

	var cfg Config
	fs.StringVar(&cfg.URL, "url", "", "URL of the service to be tested (required, http or https)")
	fs.IntVar(&cfg.Requests, "requests", 0, "total number of requests to perform (>= 1)")
	fs.IntVar(&cfg.Concurrency, "concurrency", 1, "number of simultaneous requests (>= 1)")
	fs.DurationVar(&cfg.Timeout, "timeout", DefaultTimeout, "timeout of each request (e.g. 500ms, 5s, 1m)")
	fs.IntVar(&cfg.MaxRedirects, "max-redirects", DefaultMaxRedirects, "maximum redirects to follow per request (0 = do not follow)")
	fs.Usage = func() {
		fmt.Fprintln(output, "Usage: stress-test --url=<url> --requests=<n> --concurrency=<n> [--timeout=10s] [--max-redirects=10]")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	if fs.NArg() > 0 {
		return Config{}, fmt.Errorf("unexpected arguments: %v", fs.Args())
	}
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	if cfg.Concurrency > cfg.Requests {
		cfg.Concurrency = cfg.Requests
	}
	return cfg, nil
}

func (c Config) validate() error {
	if c.URL == "" {
		return errors.New("--url is required")
	}
	u, err := url.ParseRequestURI(c.URL)
	if err != nil {
		return fmt.Errorf("--url is invalid: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("--url must use http or https scheme, got %q", u.Scheme)
	}
	if u.Host == "" {
		return errors.New("--url must contain a host")
	}
	if c.Requests < 1 {
		return fmt.Errorf("--requests must be >= 1, got %d", c.Requests)
	}
	if c.Concurrency < 1 {
		return fmt.Errorf("--concurrency must be >= 1, got %d", c.Concurrency)
	}
	if c.MaxRedirects < 0 {
		return fmt.Errorf("--max-redirects must be >= 0, got %d", c.MaxRedirects)
	}
	if c.Timeout <= 0 {
		return fmt.Errorf("--timeout must be > 0, got %s", c.Timeout)
	}
	return nil
}

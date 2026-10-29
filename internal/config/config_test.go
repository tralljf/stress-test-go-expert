package config

import (
	"io"
	"strings"
	"testing"
	"time"
)

func TestParseValid(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want Config
	}{
		{
			name: "equals form",
			args: []string{"--url=http://google.com", "--requests=1000", "--concurrency=10"},
			want: Config{URL: "http://google.com", Requests: 1000, Concurrency: 10, Timeout: DefaultTimeout, MaxRedirects: DefaultMaxRedirects},
		},
		{
			name: "space form and custom timeout",
			args: []string{"--url", "https://example.com/x", "--requests", "5", "--concurrency", "2", "--timeout", "3s", "--max-redirects", "0"},
			want: Config{URL: "https://example.com/x", Requests: 5, Concurrency: 2, Timeout: 3 * time.Second, MaxRedirects: 0},
		},
		{
			name: "concurrency capped to requests",
			args: []string{"--url=http://localhost:8080", "--requests=3", "--concurrency=50"},
			want: Config{URL: "http://localhost:8080", Requests: 3, Concurrency: 3, Timeout: DefaultTimeout, MaxRedirects: DefaultMaxRedirects},
		},
		{
			name: "default concurrency is 1",
			args: []string{"--url=http://localhost", "--requests=2"},
			want: Config{URL: "http://localhost", Requests: 2, Concurrency: 1, Timeout: DefaultTimeout, MaxRedirects: DefaultMaxRedirects},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(tc.args, io.Discard)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestParseInvalid(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"missing url", []string{"--requests=1"}, "--url is required"},
		{"url without scheme", []string{"--url=google.com", "--requests=1"}, "--url"},
		{"ftp scheme", []string{"--url=ftp://google.com", "--requests=1"}, "http or https"},
		{"url without host", []string{"--url=http://", "--requests=1"}, "host"},
		{"missing requests", []string{"--url=http://a.com"}, "--requests must be >= 1"},
		{"zero requests", []string{"--url=http://a.com", "--requests=0"}, "--requests must be >= 1"},
		{"negative requests", []string{"--url=http://a.com", "--requests=-5"}, "--requests must be >= 1"},
		{"zero concurrency", []string{"--url=http://a.com", "--requests=5", "--concurrency=0"}, "--concurrency must be >= 1"},
		{"negative concurrency", []string{"--url=http://a.com", "--requests=5", "--concurrency=-1"}, "--concurrency must be >= 1"},
		{"non numeric requests", []string{"--url=http://a.com", "--requests=abc"}, "invalid value"},
		{"zero timeout", []string{"--url=http://a.com", "--requests=1", "--timeout=0s"}, "--timeout"},
		{"negative max redirects", []string{"--url=http://a.com", "--requests=1", "--max-redirects=-1"}, "--max-redirects"},
		{"unknown flag", []string{"--url=http://a.com", "--requests=1", "--foo=1"}, "not defined"},
		{"positional args", []string{"--url=http://a.com", "--requests=1", "extra"}, "unexpected arguments"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(tc.args, io.Discard)
			if err == nil {
				t.Fatalf("expected error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %q does not contain %q", err, tc.wantErr)
			}
		})
	}
}

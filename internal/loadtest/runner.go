// Package loadtest executes the HTTP load test and builds its report.
package loadtest

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// Options configures a load test run.
type Options struct {
	URL         string
	Requests    int
	Concurrency int
}

// NewHTTPClient builds a client tuned for load testing: keep-alive enabled,
// enough idle connections per host for every worker and a per-request timeout.
//
// Redirects are followed up to maxRedirects hops and the status counted is the
// one of the final response. When the limit is reached the last redirect
// response (3xx) is returned and counted instead of being turned into an error,
// so redirect loops (e.g. rate limiting pages) still show up as HTTP statuses.
// maxRedirects = 0 disables redirect following.
func NewHTTPClient(concurrency int, timeout time.Duration, maxRedirects int) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = concurrency * 2
	transport.MaxIdleConnsPerHost = concurrency
	transport.MaxConnsPerHost = 0
	transport.IdleConnTimeout = 90 * time.Second
	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			if len(via) > maxRedirects {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
}

// Run performs exactly opts.Requests GET requests against opts.URL using
// opts.Concurrency workers and returns the aggregated report.
func Run(ctx context.Context, client *http.Client, opts Options) Report {
	workers := opts.Concurrency
	if workers > opts.Requests {
		workers = opts.Requests
	}
	if workers < 1 {
		workers = 1
	}

	var next atomic.Int64
	total := int64(opts.Requests)
	partials := make([]Report, workers)

	start := time.Now()
	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func(local *Report) {
			defer wg.Done()
			local.StatusCodes = map[int]int{}
			local.Errors = map[string]int{}
			local.ErrorSamples = map[string]string{}
			for next.Add(1) <= total {
				status, err := doRequest(ctx, client, opts.URL)
				local.Total++
				if err != nil {
					kind := classifyError(err)
					local.Errors[kind]++
					if _, ok := local.ErrorSamples[kind]; !ok {
						local.ErrorSamples[kind] = err.Error()
					}
					continue
				}
				local.StatusCodes[status]++
			}
		}(&partials[w])
	}
	wg.Wait()

	report := Report{StatusCodes: map[int]int{}, Errors: map[string]int{}, ErrorSamples: map[string]string{}}
	for _, p := range partials {
		report.merge(p)
	}
	report.Duration = time.Since(start)
	report.Concurrency = workers
	report.URL = opts.URL
	return report
}

func doRequest(ctx context.Context, client *http.Client, url string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	// Drain the body so the keep-alive connection can be reused.
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		return 0, err
	}
	return resp.StatusCode, nil
}

func classifyError(err error) string {
	var netErr net.Error
	switch {
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, os.ErrDeadlineExceeded),
		errors.As(err, &netErr) && netErr.Timeout():
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, syscall.ECONNREFUSED):
		return "connection refused"
	case errors.Is(err, syscall.ECONNRESET):
		return "connection reset"
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return "dns error"
	}
	return "other error"
}

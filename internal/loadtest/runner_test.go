package loadtest

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunExactRequestCountUnderConcurrency(t *testing.T) {
	cases := []struct {
		requests, concurrency int
	}{
		{1, 1}, {1, 10}, {7, 3}, {100, 1}, {1000, 10}, {1000, 64}, {997, 50},
	}
	for _, tc := range cases {
		var hits atomic.Int64
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			hits.Add(1)
			w.WriteHeader(http.StatusOK)
		}))

		rep := Run(context.Background(), NewHTTPClient(tc.concurrency, 5*time.Second, 10),
			Options{URL: srv.URL, Requests: tc.requests, Concurrency: tc.concurrency})
		srv.Close()

		if got := hits.Load(); got != int64(tc.requests) {
			t.Errorf("requests=%d concurrency=%d: server hits = %d", tc.requests, tc.concurrency, got)
		}
		if rep.Total != tc.requests || rep.OK() != tc.requests {
			t.Errorf("requests=%d concurrency=%d: report total=%d ok=%d", tc.requests, tc.concurrency, rep.Total, rep.OK())
		}
	}
}

func TestRunStatusDistribution(t *testing.T) {
	// The server cycles 200, 404, 500, 200, 404, 500... based on the hit order,
	// so for 900 requests each status must appear exactly 300 times.
	statuses := []int{http.StatusOK, http.StatusNotFound, http.StatusInternalServerError}
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := hits.Add(1) - 1
		w.WriteHeader(statuses[n%int64(len(statuses))])
		_, _ = w.Write([]byte("body that must be drained"))
	}))
	defer srv.Close()

	rep := Run(context.Background(), NewHTTPClient(20, 5*time.Second, 10),
		Options{URL: srv.URL, Requests: 900, Concurrency: 20})

	if hits.Load() != 900 || rep.Total != 900 {
		t.Fatalf("hits=%d total=%d, want 900", hits.Load(), rep.Total)
	}
	for _, s := range statuses {
		if rep.StatusCodes[s] != 300 {
			t.Errorf("status %d: got %d, want 300", s, rep.StatusCodes[s])
		}
	}
	if rep.TotalErrors() != 0 {
		t.Errorf("errors = %v, want none", rep.Errors)
	}
}

func TestRunFollowsRedirectsAndCountsFinalStatus(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/final", http.StatusMovedPermanently)
	})
	mux.HandleFunc("/final", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	rep := Run(context.Background(), NewHTTPClient(5, 5*time.Second, 10),
		Options{URL: srv.URL + "/start", Requests: 50, Concurrency: 5})
	if rep.Total != 50 || rep.OK() != 50 || rep.StatusCodes[http.StatusMovedPermanently] != 0 {
		t.Fatalf("unexpected report: total=%d codes=%v", rep.Total, rep.StatusCodes)
	}
}

func TestRunCountsTimeouts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	rep := Run(context.Background(), NewHTTPClient(4, 20*time.Millisecond, 10),
		Options{URL: srv.URL, Requests: 8, Concurrency: 4})
	if rep.Total != 8 || rep.Errors["timeout"] != 8 {
		t.Fatalf("total=%d errors=%v, want 8 timeouts", rep.Total, rep.Errors)
	}
}

func TestRunCountsConnectionErrors(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close() // nothing listens on this address anymore

	rep := Run(context.Background(), NewHTTPClient(3, time.Second, 10),
		Options{URL: url, Requests: 10, Concurrency: 3})
	if rep.Total != 10 || rep.TotalErrors() != 10 || len(rep.StatusCodes) != 0 {
		t.Fatalf("total=%d errors=%v codes=%v", rep.Total, rep.Errors, rep.StatusCodes)
	}
	if rep.Errors["connection refused"] != 10 {
		t.Errorf("errors = %v, want 10 connection refused", rep.Errors)
	}
}

func TestReportPrintSumMatchesTotal(t *testing.T) {
	rep := Report{
		URL: "http://example.com", Concurrency: 2, Duration: 1500 * time.Millisecond, Total: 10,
		StatusCodes: map[int]int{200: 5, 404: 2, 500: 1},
		Errors:      map[string]int{"timeout": 2},
	}
	var buf bytes.Buffer
	rep.Print(&buf)
	out := buf.String()
	for _, want := range []string{
		"Total time:               1.5s",
		"Total requests:           10",
		"HTTP 200 (OK):            5",
		"404 Not Found",
		"500 Internal Server Error",
		"Errors (no HTTP response): 2",
		"timeout",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRunRedirectLimitCountsLastRedirectStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/loop", http.StatusFound) // infinite redirect loop
	}))
	defer srv.Close()

	for _, max := range []int{0, 3} {
		rep := Run(context.Background(), NewHTTPClient(2, 5*time.Second, max),
			Options{URL: srv.URL, Requests: 10, Concurrency: 2})
		if rep.Total != 10 || rep.StatusCodes[http.StatusFound] != 10 || rep.TotalErrors() != 0 {
			t.Fatalf("max=%d: total=%d codes=%v errors=%v", max, rep.Total, rep.StatusCodes, rep.Errors)
		}
	}
}

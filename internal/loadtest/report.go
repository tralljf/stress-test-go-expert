package loadtest

import (
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// Report is the aggregated result of a load test.
type Report struct {
	URL         string
	Concurrency int
	Duration    time.Duration
	Total       int
	StatusCodes map[int]int    // final HTTP status -> count
	Errors      map[string]int // error category -> count (requests without response)
	// ErrorSamples keeps the first error message seen for each category.
	ErrorSamples map[string]string
}

func (r *Report) merge(other Report) {
	r.Total += other.Total
	for code, n := range other.StatusCodes {
		r.StatusCodes[code] += n
	}
	for kind, n := range other.Errors {
		r.Errors[kind] += n
	}
	for kind, msg := range other.ErrorSamples {
		if _, ok := r.ErrorSamples[kind]; !ok {
			r.ErrorSamples[kind] = msg
		}
	}
}

// OK returns the number of requests answered with HTTP 200.
func (r Report) OK() int { return r.StatusCodes[http.StatusOK] }

// TotalErrors returns the number of requests that got no HTTP response.
func (r Report) TotalErrors() int {
	n := 0
	for _, v := range r.Errors {
		n += v
	}
	return n
}

// Print writes the human readable report.
func (r Report) Print(w io.Writer) {
	line := strings.Repeat("=", 50)
	fmt.Fprintln(w, line)
	fmt.Fprintln(w, " STRESS TEST REPORT")
	fmt.Fprintln(w, line)
	fmt.Fprintf(w, "URL:                      %s\n", r.URL)
	fmt.Fprintf(w, "Concurrency:              %d\n", r.Concurrency)
	fmt.Fprintf(w, "Total time:               %s\n", r.Duration.Round(time.Millisecond))
	fmt.Fprintf(w, "Total requests:           %d\n", r.Total)
	if secs := r.Duration.Seconds(); secs > 0 {
		fmt.Fprintf(w, "Requests/second:          %.2f\n", float64(r.Total)/secs)
	}
	fmt.Fprintf(w, "HTTP 200 (OK):            %d\n", r.OK())

	fmt.Fprintln(w, "\nOther HTTP status codes:")
	codes := make([]int, 0, len(r.StatusCodes))
	for code := range r.StatusCodes {
		if code != http.StatusOK {
			codes = append(codes, code)
		}
	}
	sort.Ints(codes)
	if len(codes) == 0 {
		fmt.Fprintln(w, "  (none)")
	}
	for _, code := range codes {
		fmt.Fprintf(w, "  %d %-30s %d\n", code, http.StatusText(code), r.StatusCodes[code])
	}

	fmt.Fprintf(w, "\nErrors (no HTTP response): %d\n", r.TotalErrors())
	kinds := make([]string, 0, len(r.Errors))
	for k := range r.Errors {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	for _, k := range kinds {
		fmt.Fprintf(w, "  %-34s %d\n", k, r.Errors[k])
		if msg := r.ErrorSamples[k]; msg != "" {
			fmt.Fprintf(w, "    e.g.: %s\n", msg)
		}
	}
	fmt.Fprintln(w, line)
}

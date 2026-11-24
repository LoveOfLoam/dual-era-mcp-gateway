// Package load is a small in-process HTTP load generator used for local
// evidence. It is not a replacement for an Envoy/SSE/k6 benchmark.
package load

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"sync"
	"time"
)

// Config controls one run.
type Config struct {
	URL         string
	Method      string
	Body        []byte
	Headers     map[string]string
	Concurrency int
	Duration    time.Duration
	Client      *http.Client
}

// Result is the aggregate.
type Result struct {
	Requests int           `json:"requests"`
	Errors   int           `json:"errors"`
	Status   map[int]int   `json:"status"`
	Elapsed  time.Duration `json:"elapsed"`
	RPS      float64       `json:"rps"`
	P50      time.Duration `json:"p50"`
	P95      time.Duration `json:"p95"`
	P99      time.Duration `json:"p99"`
	Max      time.Duration `json:"max"`
	Samples  []string      `json:"samples,omitempty"`
}

// Run executes cfg until the duration expires or ctx is canceled.
func Run(ctx context.Context, cfg Config) (Result, error) {
	if cfg.URL == "" {
		return Result{}, errors.New("url is required")
	}
	if cfg.Method == "" {
		cfg.Method = http.MethodGet
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 10
	}
	if cfg.Duration <= 0 {
		cfg.Duration = 5 * time.Second
	}
	client := cfg.Client
	if client == nil {
		client = &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        cfg.Concurrency * 4,
				MaxIdleConnsPerHost: cfg.Concurrency * 4,
			},
		}
	}

	runCtx, cancel := context.WithTimeout(ctx, cfg.Duration)
	defer cancel()
	var (
		mu        sync.Mutex
		latencies []time.Duration
		status    = map[int]int{}
		samples   []string
		requests  int
		errCount  int
	)
	start := time.Now()
	var wg sync.WaitGroup
	for i := 0; i < cfg.Concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-runCtx.Done():
					return
				default:
				}
				req, err := http.NewRequestWithContext(runCtx, cfg.Method, cfg.URL, bytes.NewReader(cfg.Body))
				if err != nil {
					return
				}
				for k, v := range cfg.Headers {
					req.Header.Set(k, v)
				}
				t0 := time.Now()
				resp, err := client.Do(req)
				elapsed := time.Since(t0)
				mu.Lock()
				requests++
				if err != nil {
					if runCtx.Err() == nil {
						errCount++
					}
					mu.Unlock()
					continue
				}
				status[resp.StatusCode]++
				if resp.StatusCode >= 500 {
					errCount++
				}
				body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
				resp.Body.Close()
				if resp.StatusCode >= 400 && len(samples) < 3 {
					samples = append(samples, fmt.Sprintf("%d %s", resp.StatusCode, string(body)))
				}
				latencies = append(latencies, elapsed)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	elapsed := time.Since(start)
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	out := Result{Requests: requests, Errors: errCount, Status: status, Elapsed: elapsed, Samples: samples}
	if elapsed > 0 {
		out.RPS = float64(requests) / elapsed.Seconds()
	}
	if len(latencies) > 0 {
		out.P50 = percentile(latencies, 50)
		out.P95 = percentile(latencies, 95)
		out.P99 = percentile(latencies, 99)
		out.Max = latencies[len(latencies)-1]
	}
	return out, nil
}

func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Ceil(float64(len(sorted))*p/100)) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// Summary returns a human-readable one-line result.
func (r Result) Summary() string {
	return fmt.Sprintf("requests=%d errors=%d rps=%.1f p50=%s p95=%s p99=%s max=%s",
		r.Requests, r.Errors, r.RPS, r.P50, r.P95, r.P99, r.Max)
}

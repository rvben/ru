package utils

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func BenchmarkStandardHTTPClient(b *testing.B) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate a typical JSON response with some processing delay
		time.Sleep(10 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"version": "1.0.0"}`))
	}))
	defer server.Close()

	// Create a standard HTTP client
	standardClient := &http.Client{
		Timeout: 30 * time.Second,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		req, _ := http.NewRequest("GET", server.URL, nil)
		req.Header.Add("Accept", "application/json")

		resp, err := standardClient.Do(req)
		if err != nil {
			b.Fatalf("request failed: %v", err)
		}

		// Read and close body to avoid leaks
		_, _ = ReadResponseBody(resp)
	}
}

func BenchmarkOptimizedHTTPClient(b *testing.B) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate a typical JSON response with some processing delay
		time.Sleep(10 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"version": "1.0.0"}`))
	}))
	defer server.Close()

	// Create an optimized HTTP client
	optimizedClient := NewHTTPClient()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		resp, err := optimizedClient.GetWithRetry(server.URL, map[string]string{
			"Accept": "application/json",
		})
		if err != nil {
			b.Fatalf("request failed: %v", err)
		}

		// Read and close body to avoid leaks
		_, _ = ReadResponseBody(resp)
	}
}

func BenchmarkHTTPClientConcurrency(b *testing.B) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate a typical JSON response with some processing delay
		time.Sleep(10 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"version": "1.0.0"}`))
	}))
	defer server.Close()

	benchmarks := []struct {
		name           string
		concurrency    int
		useOptimized   bool
		connectionPool bool
	}{
		{"Standard-NoPool-1", 1, false, false},
		{"Standard-NoPool-10", 10, false, false},
		{"Standard-WithPool-1", 1, false, true},
		{"Standard-WithPool-10", 10, false, true},
		{"Optimized-1", 1, true, true},
		{"Optimized-10", 10, true, true},
		{"Optimized-50", 50, true, true},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			var client interface{}

			if bm.useOptimized {
				client = NewHTTPClient()
			} else {
				if bm.connectionPool {
					transport := &http.Transport{
						MaxIdleConns:        100,
						MaxIdleConnsPerHost: 10,
						IdleConnTimeout:     90 * time.Second,
					}
					client = &http.Client{
						Transport: transport,
						Timeout:   30 * time.Second,
					}
				} else {
					client = &http.Client{
						Timeout: 30 * time.Second,
					}
				}
			}

			// Reset timer to exclude setup
			b.ResetTimer()
			b.ReportAllocs()

			// Create a wait group to coordinate goroutines
			var wg sync.WaitGroup
			requestsPerGoroutine := b.N / bm.concurrency
			if requestsPerGoroutine < 1 {
				requestsPerGoroutine = 1
			}

			// Limit to maxRequestsPerGoroutine to avoid excessive memory use
			maxRequestsPerGoroutine := 1000
			if requestsPerGoroutine > maxRequestsPerGoroutine {
				requestsPerGoroutine = maxRequestsPerGoroutine
			}

			// Calculate total requests
			totalRequests := requestsPerGoroutine * bm.concurrency
			if totalRequests > b.N {
				totalRequests = b.N
			}

			// Adjust b.N to match our actual work
			b.N = totalRequests

			// Launch goroutines
			for c := 0; c < bm.concurrency; c++ {
				wg.Add(1)
				go func() {
					defer wg.Done()

					for i := 0; i < requestsPerGoroutine; i++ {
						var resp *http.Response
						var err error

						if bm.useOptimized {
							optimizedClient := client.(*OptimizerHTTPClient)
							resp, err = optimizedClient.GetWithRetry(server.URL, map[string]string{
								"Accept": "application/json",
							})
						} else {
							standardClient := client.(*http.Client)
							req, _ := http.NewRequest("GET", server.URL, nil)
							req.Header.Add("Accept", "application/json")
							resp, err = standardClient.Do(req)
						}

						if err != nil {
							b.Errorf("request failed: %v", err)
							continue
						}

						// Read and close body to avoid leaks
						_, _ = ReadResponseBody(resp)
					}
				}()
			}

			wg.Wait()
		})
	}
}

func BenchmarkHTTPClientErrorRecovery(b *testing.B) {
	// Create a test server that initially returns errors but then recovers
	errorUntil := 10
	requestCount := 0
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		currentRequest := requestCount
		requestCount++
		mu.Unlock()

		if currentRequest < errorUntil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// After errors, return normal responses
		time.Sleep(5 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"version": "1.0.0"}`))
	}))
	defer server.Close()

	benchmarks := []struct {
		name         string
		useOptimized bool
	}{
		{"Standard", false},
		{"Optimized", true},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			// Reset request counter
			mu.Lock()
			requestCount = 0
			mu.Unlock()

			// Configure client
			var standardClient *http.Client
			var optimizedClient *OptimizerHTTPClient

			if bm.useOptimized {
				// Configure for testing - shorter timeouts
				oldBackoff := InitialBackoff
				oldMaxRetries := MaxRetries
				InitialBackoff = 5 * time.Millisecond
				MaxRetries = 3

				optimizedClient = NewHTTPClient()

				defer func() {
					InitialBackoff = oldBackoff
					MaxRetries = oldMaxRetries
				}()
			} else {
				standardClient = &http.Client{
					Timeout: 5 * time.Second,
				}
			}

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				var resp *http.Response
				var err error

				if bm.useOptimized {
					// Try 3 times with optimized client that has built-in retries
					resp, err = optimizedClient.GetWithRetry(server.URL, nil)
				} else {
					// Manually retry up to 3 times with standard client
					for retry := 0; retry < 3; retry++ {
						req, _ := http.NewRequest("GET", server.URL, nil)
						resp, err = standardClient.Do(req)

						if err == nil && resp.StatusCode < 500 {
							break
						}

						if resp != nil {
							resp.Body.Close()
						}

						// Simple backoff
						time.Sleep(5 * time.Millisecond * time.Duration(retry+1))
					}
				}

				if err != nil {
					// Just log errors but continue
					b.Logf("request failed: %v", err)
					continue
				}

				if resp != nil {
					// Read and close body to avoid leaks
					_, _ = ReadResponseBody(resp)
				}
			}
		})
	}
}

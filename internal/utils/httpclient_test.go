package utils

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestHTTPClientBasicRequest(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	// Create HTTP client
	client := NewHTTPClient()

	// Send request
	resp, err := client.GetWithRetry(server.URL, map[string]string{
		"Accept": "application/json",
	})

	// Check for errors
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Check response status
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// Check metrics
	metrics := client.GetMetrics()
	if metrics.RequestCount != 1 {
		t.Errorf("Expected 1 request, got %d", metrics.RequestCount)
	}
	if metrics.SuccessCount != 1 {
		t.Errorf("Expected 1 success, got %d", metrics.SuccessCount)
	}
	if metrics.FailureCount != 0 {
		t.Errorf("Expected 0 failures, got %d", metrics.FailureCount)
	}
}

func TestHTTPClientRetry(t *testing.T) {
	// Counter for number of requests
	var requestCount int32

	// Create a test server that fails initially but succeeds on retry
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&requestCount, 1)
		if count <= 2 {
			// Fail the first two requests with a 500 error
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		// Succeed on the third request
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	// Create HTTP client with shorter backoff for testing
	oldInitialBackoff := InitialBackoff
	oldMaxRetries := MaxRetries
	InitialBackoff = 10 * time.Millisecond
	MaxRetries = 3
	defer func() {
		InitialBackoff = oldInitialBackoff
		MaxRetries = oldMaxRetries
	}()

	client := NewHTTPClient()

	// Send request
	resp, err := client.GetWithRetry(server.URL, nil)

	// Check results
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
	if atomic.LoadInt32(&requestCount) != 3 {
		t.Errorf("Expected 3 requests, got %d", requestCount)
	}

	// Check metrics
	metrics := client.GetMetrics()
	if metrics.RequestCount != 1 {
		t.Errorf("Expected 1 request, got %d", metrics.RequestCount)
	}
	if metrics.RetryCount != 2 {
		t.Errorf("Expected 2 retries, got %d", metrics.RetryCount)
	}
}

func TestHTTPClientCircuitBreaker(t *testing.T) {
	// Create a test server that always fails
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	// Set circuit breaker threshold to a low value for testing
	oldThreshold := CircuitBreakerThreshold
	oldMaxRetries := MaxRetries
	oldInitialBackoff := InitialBackoff
	CircuitBreakerThreshold = 2
	MaxRetries = 1
	InitialBackoff = 5 * time.Millisecond
	defer func() {
		CircuitBreakerThreshold = oldThreshold
		MaxRetries = oldMaxRetries
		InitialBackoff = oldInitialBackoff
	}()

	client := NewHTTPClient()

	// First request - should fail but not trip circuit breaker
	_, err1 := client.GetWithRetry(server.URL, nil)
	if err1 == nil {
		t.Fatal("Expected error, got nil")
	}

	// Second request - should trip the circuit breaker
	_, err2 := client.GetWithRetry(server.URL, nil)
	if err2 == nil {
		t.Fatal("Expected error, got nil")
	}

	// Third request - circuit breaker should be open
	_, err3 := client.GetWithRetry(server.URL, nil)
	if err3 == nil {
		t.Fatal("Expected error due to open circuit breaker, got nil")
	}
	if err3.Error() != "circuit breaker open: too many recent failures" {
		t.Errorf("Expected circuit breaker error, got: %v", err3)
	}

	// Check metrics
	metrics := client.GetMetrics()
	if metrics.CircuitBreakerTrips != 1 {
		t.Errorf("Expected 1 circuit breaker trip, got %d", metrics.CircuitBreakerTrips)
	}
}

func TestHTTPClientRateLimiting(t *testing.T) {
	// Counter for number of requests
	var requestCount int32

	// Create a test server that simulates rate limiting
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&requestCount, 1)
		if count <= 2 {
			// Return rate limit error for first two requests
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":"rate limit exceeded"}`))
			return
		}
		// Succeed on the third request
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	// Create HTTP client with shorter backoff for testing
	oldInitialBackoff := InitialBackoff
	oldMaxRetries := MaxRetries
	InitialBackoff = 5 * time.Millisecond
	MaxRetries = 3
	defer func() {
		InitialBackoff = oldInitialBackoff
		MaxRetries = oldMaxRetries
	}()

	client := NewHTTPClient()

	// Send request
	resp, err := client.GetWithRetry(server.URL, nil)

	// Check results
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
	if atomic.LoadInt32(&requestCount) != 3 {
		t.Errorf("Expected 3 requests, got %d", requestCount)
	}

	// Check metrics
	metrics := client.GetMetrics()
	if metrics.RetryCount != 2 {
		t.Errorf("Expected 2 retries, got %d", metrics.RetryCount)
	}
}

func BenchmarkHTTPClientConcurrentRequests(b *testing.B) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate some processing time
		time.Sleep(5 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	// Create HTTP client
	client := NewHTTPClient()

	// Reset benchmarking timer to exclude setup costs
	b.ResetTimer()

	// Run benchmark with concurrent requests
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			resp, err := client.GetWithRetry(server.URL, nil)
			if err != nil {
				b.Fatalf("Unexpected error: %v", err)
			}
			if resp.StatusCode != http.StatusOK {
				b.Fatalf("Unexpected status code: %d", resp.StatusCode)
			}
			// Always read and close the body
			_, _ = ReadResponseBody(resp)
		}
	})
}

func BenchmarkHTTPClientSequentialRequests(b *testing.B) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate some processing time
		time.Sleep(5 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	// Create HTTP client
	client := NewHTTPClient()

	// Reset benchmarking timer to exclude setup costs
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		resp, err := client.GetWithRetry(server.URL, nil)
		if err != nil {
			b.Fatalf("Unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			b.Fatalf("Unexpected status code: %d", resp.StatusCode)
		}
		// Always read and close the body
		_, _ = ReadResponseBody(resp)
	}
}

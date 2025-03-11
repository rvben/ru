package utils

import (
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	// Default connection pool settings
	MaxIdleConnsPerHost     = 10
	MaxIdleConns            = 100
	IdleConnTimeout         = 90 * time.Second
	TLSHandshakeTimeout     = 10 * time.Second
	ExpectContinueTimeout   = 1 * time.Second
	ConnectionTimeout       = 30 * time.Second
	ResponseTimeout         = 30 * time.Second
	KeepAliveTimeout        = 30 * time.Second
	MaxRetries              = 3
	InitialBackoff          = 100 * time.Millisecond
	MaxBackoff              = 5 * time.Second
	BackoffMultiplier       = 2.0
	BackoffJitter           = 0.2
	CircuitBreakerThreshold = 5                // Number of failures before tripping
	CircuitBreakerResetTime = 30 * time.Second // Time before attempting to reset
)

// HTTPClientMetrics tracks metrics for the HTTP client
type HTTPClientMetrics struct {
	RequestCount        int64
	SuccessCount        int64
	FailureCount        int64
	RetryCount          int64
	TotalRequestTime    int64 // in nanoseconds
	CircuitBreakerTrips int64
	lock                sync.RWMutex
	failureTimestamps   []time.Time
}

// CircuitBreaker implements a simple circuit breaker pattern
type CircuitBreaker struct {
	metrics   *HTTPClientMetrics
	state     int32 // 0 = closed, 1 = open
	lastTrip  time.Time
	lock      sync.RWMutex
	hostState map[string]hostCircuitState // Track circuit state per host
}

type hostCircuitState struct {
	state    int32     // 0 = closed, 1 = open
	lastTrip time.Time // when the circuit for this host was last tripped
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(metrics *HTTPClientMetrics) *CircuitBreaker {
	return &CircuitBreaker{
		metrics:   metrics,
		state:     0, // Closed by default
		hostState: make(map[string]hostCircuitState),
	}
}

// IsOpen checks if the circuit breaker is open
func (cb *CircuitBreaker) IsOpen() bool {
	// Global circuit breaker check
	if atomic.LoadInt32(&cb.state) == 1 {
		cb.lock.RLock()
		elapsed := time.Since(cb.lastTrip)
		cb.lock.RUnlock()

		if elapsed > CircuitBreakerResetTime {
			return false // Allow a test request
		}
		return true
	}
	return false
}

// IsHostOpen checks if the circuit breaker for a specific host is open
func (cb *CircuitBreaker) IsHostOpen(host string) bool {
	// First check global circuit breaker
	if cb.IsOpen() {
		return true
	}

	// Then check host-specific circuit breaker
	cb.lock.RLock()
	hostCircuit, exists := cb.hostState[host]
	cb.lock.RUnlock()

	if !exists {
		return false
	}

	if atomic.LoadInt32(&hostCircuit.state) == 1 {
		cb.lock.RLock()
		elapsed := time.Since(hostCircuit.lastTrip)
		cb.lock.RUnlock()

		if elapsed > CircuitBreakerResetTime {
			return false // Allow a test request for this host
		}
		return true
	}

	return false
}

// Trip opens the circuit breaker
func (cb *CircuitBreaker) Trip() {
	if atomic.CompareAndSwapInt32(&cb.state, 0, 1) {
		cb.lock.Lock()
		cb.lastTrip = time.Now()
		cb.lock.Unlock()
		atomic.AddInt64(&cb.metrics.CircuitBreakerTrips, 1)
		Debug("http", "Circuit breaker tripped")
	}
}

// TripHost opens the circuit breaker for a specific host
func (cb *CircuitBreaker) TripHost(host string) {
	cb.lock.Lock()
	hostCircuit, exists := cb.hostState[host]
	if !exists {
		hostCircuit = hostCircuitState{
			state: 0,
		}
	}

	if atomic.CompareAndSwapInt32(&hostCircuit.state, 0, 1) {
		hostCircuit.lastTrip = time.Now()
		cb.hostState[host] = hostCircuit
		atomic.AddInt64(&cb.metrics.CircuitBreakerTrips, 1)
		Debug("http", "Circuit breaker tripped for host: %s", host)
	}
	cb.lock.Unlock()
}

// Reset closes the circuit breaker
func (cb *CircuitBreaker) Reset() {
	atomic.StoreInt32(&cb.state, 0)
	Debug("http", "Circuit breaker reset")
}

// ResetHost closes the circuit breaker for a specific host
func (cb *CircuitBreaker) ResetHost(host string) {
	cb.lock.Lock()
	hostCircuit, exists := cb.hostState[host]
	if exists {
		atomic.StoreInt32(&hostCircuit.state, 0)
		cb.hostState[host] = hostCircuit
		Debug("http", "Circuit breaker reset for host: %s", host)
	}
	cb.lock.Unlock()
}

// RecordSuccess records a successful request
func (m *HTTPClientMetrics) RecordSuccess() {
	atomic.AddInt64(&m.SuccessCount, 1)
}

// RecordFailure records a failed request
func (m *HTTPClientMetrics) RecordFailure() {
	atomic.AddInt64(&m.FailureCount, 1)

	// Add timestamp to failure history
	m.lock.Lock()
	now := time.Now()
	m.failureTimestamps = append(m.failureTimestamps, now)

	// Clean up old timestamps (more than 1 minute old)
	cutoff := now.Add(-1 * time.Minute)
	newTimestamps := make([]time.Time, 0, len(m.failureTimestamps))
	for _, ts := range m.failureTimestamps {
		if ts.After(cutoff) {
			newTimestamps = append(newTimestamps, ts)
		}
	}
	m.failureTimestamps = newTimestamps
	m.lock.Unlock()
}

// RecordRetry records a retry attempt
func (m *HTTPClientMetrics) RecordRetry() {
	atomic.AddInt64(&m.RetryCount, 1)
}

// RecentFailureCount gets the number of failures in the last minute
func (m *HTTPClientMetrics) RecentFailureCount() int {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return len(m.failureTimestamps)
}

// OptimizerHTTPClient provides an HTTP client with optimized settings
type OptimizerHTTPClient struct {
	client         *http.Client
	metrics        *HTTPClientMetrics
	circuitBreaker *CircuitBreaker
}

// NewHTTPClient creates a new HTTP client with optimized settings
func NewHTTPClient() *OptimizerHTTPClient {
	metrics := &HTTPClientMetrics{
		failureTimestamps: make([]time.Time, 0, 10),
	}

	cb := NewCircuitBreaker(metrics)

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   ConnectionTimeout,
			KeepAlive: KeepAliveTimeout,
		}).DialContext,
		MaxIdleConns:          MaxIdleConns,
		IdleConnTimeout:       IdleConnTimeout,
		TLSHandshakeTimeout:   TLSHandshakeTimeout,
		ExpectContinueTimeout: ExpectContinueTimeout,
		MaxIdleConnsPerHost:   MaxIdleConnsPerHost,
		// Enable HTTP/2 for better performance
		ForceAttemptHTTP2: true,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   ResponseTimeout,
	}

	return &OptimizerHTTPClient{
		client:         client,
		metrics:        metrics,
		circuitBreaker: cb,
	}
}

// GetWithRetry performs an HTTP GET with retry and circuit breaker
func (c *OptimizerHTTPClient) GetWithRetry(urlStr string, headers map[string]string) (*http.Response, error) {
	atomic.AddInt64(&c.metrics.RequestCount, 1)
	startTime := time.Now()

	defer func() {
		elapsed := time.Since(startTime)
		atomic.AddInt64(&c.metrics.TotalRequestTime, elapsed.Nanoseconds())
	}()

	// Extract host from URL for host-specific circuit breaking
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}
	host := parsedURL.Host

	// Check circuit breaker for this specific host
	if c.circuitBreaker.IsHostOpen(host) {
		return nil, fmt.Errorf("circuit breaker open for host %s: too many recent failures", host)
	}

	var resp *http.Response
	var lastErr error

	backoff := InitialBackoff
	hostUnreachable := false

	for attempt := 0; attempt <= MaxRetries; attempt++ {
		if attempt > 0 {
			// Add jitter to avoid thundering herd
			jitter := 1.0 + (rand.Float64()*2-1)*BackoffJitter
			sleepTime := time.Duration(float64(backoff) * jitter)

			// Log retry
			Debug("http", "Retrying request to %s (attempt %d/%d) after %v", urlStr, attempt, MaxRetries, sleepTime)
			time.Sleep(sleepTime)

			c.metrics.RecordRetry()

			// Increase backoff for next attempt
			backoff = time.Duration(math.Min(
				float64(backoff)*BackoffMultiplier,
				float64(MaxBackoff),
			))
		}

		req, err := http.NewRequest("GET", urlStr, nil)
		if err != nil {
			lastErr = err
			continue
		}

		// Add headers
		for key, value := range headers {
			req.Header.Add(key, value)
		}

		// Always add these headers for better performance
		req.Header.Add("Connection", "keep-alive")
		req.Header.Add("Accept-Encoding", "gzip, deflate")

		resp, err = c.client.Do(req)

		if err != nil {
			Debug("http", "Request error: %v", err)
			c.metrics.RecordFailure()
			lastErr = err

			// Check for host unreachable errors (DNS failures, connection refused, etc.)
			if isHostUnreachableError(err) {
				Debug("http", "Host unreachable error detected for %s: %v", host, err)
				hostUnreachable = true
				// Trip the circuit breaker immediately for this host
				c.circuitBreaker.TripHost(host)
				break // Don't retry for host unreachable errors
			}

			continue
		}

		// Check for server errors (5xx)
		if resp.StatusCode >= 500 {
			Debug("http", "Server error: %s", resp.Status)
			resp.Body.Close()
			c.metrics.RecordFailure()
			continue
		}

		// Client errors (4xx) are not retried (except 429)
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			if resp.StatusCode == 429 {
				// Rate limited, retry with backoff
				resp.Body.Close()
				c.metrics.RecordFailure()
				continue
			}

			c.metrics.RecordFailure()
			return resp, nil // Return the error response
		}

		// Success
		c.metrics.RecordSuccess()

		// Reset circuit breaker on success
		if atomic.LoadInt32(&c.circuitBreaker.state) == 1 {
			c.circuitBreaker.Reset()
		}
		// Reset host-specific circuit breaker
		c.circuitBreaker.ResetHost(host)

		return resp, nil
	}

	// If we've reached this point, all retries failed
	// Check if we should trip the circuit breaker
	if hostUnreachable || c.metrics.RecentFailureCount() >= CircuitBreakerThreshold {
		c.circuitBreaker.TripHost(host)
	}

	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}

	return nil, fmt.Errorf("all retries failed for %s: %w", host, lastErr)
}

// isHostUnreachableError checks if an error indicates that a host is completely unreachable
func isHostUnreachableError(err error) bool {
	if err == nil {
		return false
	}

	errString := err.Error()

	// Common error patterns that indicate a host is unreachable
	unreachablePatterns := []string{
		"no such host",
		"dial tcp: lookup",
		"connection refused",
		"host is down",
		"network is unreachable",
		"i/o timeout",
		"no route to host",
		"host not found",
	}

	for _, pattern := range unreachablePatterns {
		if strings.Contains(strings.ToLower(errString), pattern) {
			return true
		}
	}

	// Check for specific network error types
	var dnsErr *net.DNSError
	var opErr *net.OpError

	if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
		return true
	}

	if errors.As(err, &opErr) && (opErr.Timeout() || opErr.Temporary()) {
		return true
	}

	return false
}

// GetMetrics returns the current HTTP client metrics
func (c *OptimizerHTTPClient) GetMetrics() HTTPClientMetrics {
	return HTTPClientMetrics{
		RequestCount:        atomic.LoadInt64(&c.metrics.RequestCount),
		SuccessCount:        atomic.LoadInt64(&c.metrics.SuccessCount),
		FailureCount:        atomic.LoadInt64(&c.metrics.FailureCount),
		RetryCount:          atomic.LoadInt64(&c.metrics.RetryCount),
		TotalRequestTime:    atomic.LoadInt64(&c.metrics.TotalRequestTime),
		CircuitBreakerTrips: atomic.LoadInt64(&c.metrics.CircuitBreakerTrips),
	}
}

// ReadResponseBody reads the response body and closes it
func ReadResponseBody(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

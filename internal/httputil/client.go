// Package httputil provides HTTP client abstractions for testability.
package httputil

import (
	"bytes"
	"io"
	"net/http"
	"sync"
)

// HTTPClient abstracts HTTP operations for testability.
// Use http.DefaultClient or http.Client for production; MockHTTPClient for testing.
type HTTPClient interface {
	// Do sends an HTTP request and returns an HTTP response.
	Do(req *http.Request) (*http.Response, error)
	// Get issues a GET to the specified URL.
	Get(url string) (*http.Response, error)
	// Post issues a POST to the specified URL.
	Post(url, contentType string, body io.Reader) (*http.Response, error)
}

// StandardClient wraps *http.Client to implement HTTPClient.
type StandardClient struct {
	*http.Client
}

// NewStandardClient creates a new StandardClient wrapping the given http.Client.
func NewStandardClient(c *http.Client) *StandardClient {
	if c == nil {
		c = http.DefaultClient
	}
	return &StandardClient{Client: c}
}

// Do sends an HTTP request.
func (c *StandardClient) Do(req *http.Request) (*http.Response, error) {
	return c.Client.Do(req)
}

// Get issues a GET request.
func (c *StandardClient) Get(url string) (*http.Response, error) {
	return c.Client.Get(url)
}

// Post issues a POST request.
func (c *StandardClient) Post(url, contentType string, body io.Reader) (*http.Response, error) {
	return c.Client.Post(url, contentType, body)
}

// MockHTTPClient provides a testable HTTP client implementation.
type MockHTTPClient struct {
	mu           sync.Mutex
	DoFunc       func(req *http.Request) (*http.Response, error)
	Requests     []*http.Request
	Responses    []*MockResponse
	responseIdx  int
	DefaultError error
}

// MockResponse defines a canned HTTP response for testing.
type MockResponse struct {
	StatusCode int
	Body       string
	Headers    http.Header
	Error      error
}

// NewMockHTTPClient creates a new mock HTTP client.
func NewMockHTTPClient() *MockHTTPClient {
	return &MockHTTPClient{
		Requests:  []*http.Request{},
		Responses: []*MockResponse{},
	}
}

// AddResponse queues a response to be returned by subsequent requests.
func (m *MockHTTPClient) AddResponse(statusCode int, body string) *MockHTTPClient {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Responses = append(m.Responses, &MockResponse{
		StatusCode: statusCode,
		Body:       body,
		Headers:    make(http.Header),
	})
	return m
}

// AddErrorResponse queues an error response.
func (m *MockHTTPClient) AddErrorResponse(err error) *MockHTTPClient {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Responses = append(m.Responses, &MockResponse{Error: err})
	return m
}

// Do records the request and returns the next queued response.
func (m *MockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Requests = append(m.Requests, req)

	// Use custom DoFunc if provided
	if m.DoFunc != nil {
		return m.DoFunc(req)
	}

	// Return default error if set
	if m.DefaultError != nil {
		return nil, m.DefaultError
	}

	// Return next queued response
	if m.responseIdx < len(m.Responses) {
		resp := m.Responses[m.responseIdx]
		m.responseIdx++

		if resp.Error != nil {
			return nil, resp.Error
		}

		return &http.Response{
			StatusCode: resp.StatusCode,
			Body:       io.NopCloser(bytes.NewBufferString(resp.Body)),
			Header:     resp.Headers,
			Request:    req,
		}, nil
	}

	// Default response
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString("")),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

// Get issues a GET request.
func (m *MockHTTPClient) Get(url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return m.Do(req)
}

// Post issues a POST request.
func (m *MockHTTPClient) Post(url, contentType string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	return m.Do(req)
}

// GetRequest returns the nth recorded request.
func (m *MockHTTPClient) GetRequest(n int) *http.Request {
	m.mu.Lock()
	defer m.mu.Unlock()
	if n < 0 || n >= len(m.Requests) {
		return nil
	}
	return m.Requests[n]
}

// RequestCount returns the number of recorded requests.
func (m *MockHTTPClient) RequestCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.Requests)
}

// Reset clears all recorded requests and responses.
func (m *MockHTTPClient) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Requests = []*http.Request{}
	m.Responses = []*MockResponse{}
	m.responseIdx = 0
	m.DefaultError = nil
	m.DoFunc = nil
}

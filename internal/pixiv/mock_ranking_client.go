package pixiv

import (
	"bytes"
	"io"
	"net/http"
)

type MockRankingClient struct {
	resp         *http.Response
	err          error
	capturedReq  *http.Request
	capturedPath string
	data         []byte
	contentType  string
}

// Do records request info and returns a configurable mock HTTP response.
func (m *MockRankingClient) Do(req *http.Request) (*http.Response, error) {
	m.capturedReq = req
	if req.URL != nil {
		m.capturedPath = req.URL.RawQuery
	}

	return &http.Response{
		StatusCode: m.resp.StatusCode,
		Body:       io.NopCloser(bytes.NewReader(m.data)),
		Header:     http.Header{"Content-Type": []string{m.contentType}},
	}, m.err
}

type MockRankingClientOption func(*MockRankingClient)

// WithMockRankingStatusCode sets the response status code for the mock client.
func WithMockRankingStatusCode(code int) MockRankingClientOption {
	return func(m *MockRankingClient) {
		m.resp.StatusCode = code
	}
}

// WithMockRankingBody sets the response body for the mock client.
func WithMockRankingBody(body []byte) MockRankingClientOption {
	return func(m *MockRankingClient) {
		m.data = body
	}
}

// WithMockRankingContentType sets the content type header for the mock client.
func WithMockRankingContentType(ct string) MockRankingClientOption {
	return func(m *MockRankingClient) {
		m.contentType = ct
	}
}

// NewMockRankingClient creates a mock ranking HTTP client for tests.
func NewMockRankingClient(opts ...MockRankingClientOption) *MockRankingClient {
	m := &MockRankingClient{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader(nil)),
		},
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// CapturedQuery returns the last captured request query string.
func (m *MockRankingClient) CapturedQuery() string {
	return m.capturedPath
}

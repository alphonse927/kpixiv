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

func WithMockRankingStatusCode(code int) MockRankingClientOption {
	return func(m *MockRankingClient) {
		m.resp.StatusCode = code
	}
}

func WithMockRankingBody(body []byte) MockRankingClientOption {
	return func(m *MockRankingClient) {
		m.data = body
	}
}

func WithMockRankingContentType(ct string) MockRankingClientOption {
	return func(m *MockRankingClient) {
		m.contentType = ct
	}
}

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

func (m *MockRankingClient) CapturedQuery() string {
	return m.capturedPath
}

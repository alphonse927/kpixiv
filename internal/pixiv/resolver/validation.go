package resolver

import (
	"bytes"
	"io"
	"net/http"
	"strings"
)

// IsImageResponse reports whether a response is a successful image response.
func IsImageResponse(resp *http.Response) bool {
	if resp.StatusCode != http.StatusOK {
		return false
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		return false
	}

	if strings.Contains(contentType, "text/html") {
		return false
	}

	if !strings.HasPrefix(contentType, "image/") {
		return false
	}

	return true
}

// HasContent reports whether the response body contains at least one byte.
func HasContent(resp *http.Response) bool {
	if resp.Body == nil {
		return false
	}

	b := make([]byte, 1)
	n, err := resp.Body.Read(b)
	if n > 0 {
		resp.Body = io.NopCloser(io.MultiReader(bytes.NewReader(b[:n]), resp.Body))
		return true
	}

	return err == nil
}

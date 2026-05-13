package resolver

import (
	"io"
	"net/http"
	"strings"
)

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

func IsHTMLResponse(resp *http.Response) bool {
	contentType := resp.Header.Get("Content-Type")
	return strings.Contains(contentType, "text/html")
}

func HasContent(resp *http.Response) bool {
	if resp.ContentLength <= 0 {
		return false
	}

	lr := io.LimitReader(resp.Body, 1)
	buf := make([]byte, 1)
	n, _ := lr.Read(buf)
	return n > 0
}

func GetContentType(resp *http.Response) string {
	return resp.Header.Get("Content-Type")
}

func GetStatusCode(resp *http.Response) int {
	return resp.StatusCode
}

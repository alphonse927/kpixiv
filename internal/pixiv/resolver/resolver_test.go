package resolver

import (
	"context"
	"testing"
)

func TestTransformThumbnailToOriginalBase(t *testing.T) {
	tests := []struct {
		name         string
		thumbnailURL string
		expected     string
		expectError  bool
	}{
		{
			name:         "standard jpg thumbnail",
			thumbnailURL: "https://i.pximg.net/c/240x480/img-master/img/2020/02/19/00/00/39/79583564_p0_master1200.jpg",
			expected:     "https://i.pximg.net/img-original/img/2020/02/19/00/00/39/79583564_p0",
			expectError:  false,
		},
		{
			name:         "png thumbnail",
			thumbnailURL: "https://i.pximg.net/c/240x480/img-master/img/2021/05/20/12/34/56/12345678_p0_master1200.png",
			expected:     "https://i.pximg.net/img-original/img/2021/05/20/12/34/56/12345678_p0",
			expectError:  false,
		},
		{
			name:         "multiple pages",
			thumbnailURL: "https://i.pximg.net/c/240x480/img-master/img/2022/01/15/08/30/00/99999999_p1_master1200.jpg",
			expected:     "https://i.pximg.net/img-original/img/2022/01/15/08/30/00/99999999_p1",
			expectError:  false,
		},
		{
			name:         "invalid URL - no img-master",
			thumbnailURL: "https://i.pximg.net/c/240x480/img-original/img/2020/02/19/00/00/39/79583564_p0.jpg",
			expected:     "",
			expectError:  true,
		},
		{
			name:         "bookmark API format with webp suffix",
			thumbnailURL: "https://i.pximg.net/c/600x1200_90_webp/img-master/img/2026/06/17/11/34/00/146094844_p0_master1200.jpg",
			expected:     "https://i.pximg.net/img-original/img/2026/06/17/11/34/00/146094844_p0",
			expectError:  false,
		},
		{
			name:         "already original URL",
			thumbnailURL: "https://i.pximg.net/img-original/img/2020/02/19/00/00/39/79583564_p0.jpg",
			expected:     "",
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := transformThumbnailToOriginalBase(tt.thumbnailURL)

			if tt.expectError && err == nil {
				t.Errorf("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tt.expectError && result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		input    string
		max      int
		expected string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "hello..."},
		{"abc", 3, "abc"},
		{"", 5, ""},
		{"test", 0, ""},
		{"verylongstring", 4, "very..."},
	}

	for _, tt := range tests {
		result := TruncateString(tt.input, tt.max)
		if result != tt.expected {
			t.Errorf("TruncateString(%q, %d) = %q, expected %q", tt.input, tt.max, result, tt.expected)
		}
	}
}

func TestResolverIntegration(t *testing.T) {
	t.Skip("Skipping integration test - requires network access to Pixiv")

	ctx := context.Background()
	resolver, err := NewResolver()
	if err != nil {
		t.Fatalf("Failed to create resolver: %v", err)
	}

	thumbnailURL := "https://i.pximg.net/c/240x480/img-master/img/2020/02/19/00/00/39/79583564_p0_master1200.jpg"

	originalURL, err := resolver.ResolveOriginalURL(ctx, thumbnailURL)
	if err != nil {
		t.Logf("Expected failure in test environment: %v", err)
		return
	}

	t.Logf("Resolved original URL: %s", originalURL)
}

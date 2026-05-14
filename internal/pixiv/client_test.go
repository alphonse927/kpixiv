package pixiv

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/alphonse927/kpixiv/internal/config"
	"github.com/alphonse927/kpixiv/internal/logger"
)

func TestMain(m *testing.M) {
	logger.Init(false)
	os.Exit(m.Run())
}

func TestParseNextPage(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected int
	}{
		{"float64 2", float64(2), 2},
		{"float64 0", float64(0), 1},
		{"float64 negative", float64(-1), 1},
		{"int 3", 3, 3},
		{"int 0", 0, 1},
		{"int negative", -5, 1},
		{"string", "invalid", 1},
		{"nil", nil, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseNextPage(tt.input)
			if got != tt.expected {
				t.Errorf("parseNextPage(%v) = %d, want %d", tt.input, got, tt.expected)
			}
		})
	}
}

func TestFetchRankingNonJSON(t *testing.T) {
	body := []byte("<!DOCTYPE html><html>Cloudflare challenge</html>")
	mockClient := NewMockRankingClient(
		WithMockRankingStatusCode(http.StatusOK),
		WithMockRankingBody(body),
		WithMockRankingContentType("text/html"),
	)

	client := &Client{rankingClient: mockClient}

	images, page, err := client.FetchRanking(context.Background(), config.RankingDailyMode.String(), 1, false)
	if err == nil {
		t.Error("FetchRanking() with HTML response: got nil error, want error")
	}

	if images != nil {
		t.Errorf("FetchRanking() with HTML response: images should be nil, got %v", images)
	}

	if page != 1 {
		t.Errorf("FetchRanking() with HTML response: page should be 1, got %d", page)
	}
}

func TestFetchRankingJSON(t *testing.T) {
	resp := RankingResponse{
		Contents: []RankingContent{
			{
				Title:    "Test Art",
				URL:      "https://i.pximg.net/img-master/img/2020/01/01/00/00/00/12345_p0_master1200.jpg",
				UserName: "TestArtist",
				IllustID: 12345,
				UserID:   99999,
				Width:    1920,
				Height:   1080,
				IsMasked: false,
			},
			{
				Title:    "Masked Art",
				URL:      "https://i.pximg.net/img-master/img/2020/01/01/00/00/00/12346_p0_master1200.jpg",
				UserName: "AnotherArtist",
				IllustID: 12346,
				UserID:   88888,
				Width:    1920,
				Height:   1080,
				IsMasked: true,
			},
		},
		Next: float64(2),
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("json.Marshal() returned error: %v", err)
	}

	mockClient := NewMockRankingClient(
		WithMockRankingStatusCode(http.StatusOK),
		WithMockRankingBody(data),
		WithMockRankingContentType("application/json"),
	)

	client := &Client{rankingClient: mockClient}

	images, nextPage, err := client.FetchRanking(context.Background(), config.RankingDailyMode.String(), 1, false)
	if err != nil {
		t.Fatalf("FetchRanking() returned error: %v", err)
	}

	if len(images) != 1 {
		t.Errorf("FetchRanking() len: got %d, want 1 (masked should be filtered)", len(images))
	}

	if images[0].ID != "12345" {
		t.Errorf("FetchRanking() first image ID: got %s, want 12345", images[0].ID)
	}

	if images[0].Artist != "TestArtist" {
		t.Errorf("FetchRanking() first image Artist: got %s, want TestArtist", images[0].Artist)
	}

	if nextPage != 2 {
		t.Errorf("FetchRanking() nextPage: got %d, want 2", nextPage)
	}
}

func TestFetchRankingWithR18Suffix(t *testing.T) {
	resp := RankingResponse{Contents: []RankingContent{}}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("json.Marshal() returned error: %v", err)
	}

	mockClient := NewMockRankingClient(
		WithMockRankingStatusCode(http.StatusOK),
		WithMockRankingBody(data),
		WithMockRankingContentType("application/json"),
	)

	client := &Client{rankingClient: mockClient}

	_, _, err = client.FetchRanking(context.Background(), config.RankingDailyMode.String(), 1, true)
	if err != nil {
		t.Fatalf("FetchRanking() returned error: %v", err)
	}

	if !strings.Contains(mockClient.CapturedQuery(), "mode="+config.RankingDailyMode.String()+"_r18") {
		t.Errorf("FetchRanking() r18=true should append _r18, got query: %s", mockClient.CapturedQuery())
	}
}

func TestFetchRankingWithR18False(t *testing.T) {
	resp := RankingResponse{Contents: []RankingContent{}}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("json.Marshal() returned error: %v", err)
	}

	mockClient := NewMockRankingClient(
		WithMockRankingStatusCode(http.StatusOK),
		WithMockRankingBody(data),
		WithMockRankingContentType("application/json"),
	)

	client := &Client{rankingClient: mockClient}

	_, _, err = client.FetchRanking(context.Background(), config.RankingWeeklyMode.String(), 1, false)
	if err != nil {
		t.Fatalf("FetchRanking() returned error: %v", err)
	}

	if !strings.Contains(mockClient.CapturedQuery(), "mode="+config.RankingWeeklyMode.String()) {
		t.Errorf("FetchRanking() r18=false should use weekly mode, got query: %s", mockClient.CapturedQuery())
	}

	if strings.Contains(mockClient.CapturedQuery(), "r18") {
		t.Errorf("FetchRanking() r18=false should not have r18 in query, got query: %s", mockClient.CapturedQuery())
	}
}

func TestFetchRankingMasksR18(t *testing.T) {
	resp := RankingResponse{
		Contents: []RankingContent{
			{IllustID: 1, Width: 1920, Height: 1080, IsMasked: true},
			{IllustID: 2, Width: 1920, Height: 1080, IsMasked: false},
		},
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("json.Marshal() returned error: %v", err)
	}

	mockClient := NewMockRankingClient(
		WithMockRankingStatusCode(http.StatusOK),
		WithMockRankingBody(data),
		WithMockRankingContentType("application/json"),
	)

	client := &Client{rankingClient: mockClient}

	images, _, err := client.FetchRanking(context.Background(), config.RankingDailyMode.String(), 1, false)
	if err != nil {
		t.Fatalf("FetchRanking() returned error: %v", err)
	}

	if len(images) != 1 {
		t.Errorf("FetchRanking() r18=false should filter masked: got %d, want 1", len(images))
	}

	imagesR18, _, err := client.FetchRanking(context.Background(), config.RankingDailyMode.String(), 1, true)
	if err != nil {
		t.Fatalf("FetchRanking() r18=true returned error: %v", err)
	}
	if len(imagesR18) != 2 {
		t.Errorf("FetchRanking() r18=true should NOT filter masked: got %d, want 2", len(imagesR18))
	}
}

func TestFetchRankingError(t *testing.T) {
	mockClient := NewMockRankingClient(
		WithMockRankingStatusCode(http.StatusInternalServerError),
		WithMockRankingBody(nil),
	)

	client := &Client{rankingClient: mockClient}

	_, _, err := client.FetchRanking(context.Background(), config.RankingDailyMode.String(), 1, false)
	if err == nil {
		t.Error("FetchRanking() with 500: got nil, want error")
	}
}

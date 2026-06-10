package pixiv

import "testing"

func TestExtractAuthParamsFromURL(t *testing.T) {
	code, state, err := extractAuthParams("https://app-api.pixiv.net/web/v1/users/auth/pixiv/callback?state=abc&code=token123")
	if err != nil {
		t.Fatalf("extractAuthParams() returned error: %v", err)
	}

	if code != "token123" {
		t.Fatalf("extractAuthParams() code = %q, want %q", code, "token123")
	}

	if state != "abc" {
		t.Fatalf("extractAuthParams() state = %q, want %q", state, "abc")
	}
}

func TestExtractAuthParamsRawValue(t *testing.T) {
	code, state, err := extractAuthParams("token123")
	if err != nil {
		t.Fatalf("extractAuthParams() returned error: %v", err)
	}

	if code != "token123" {
		t.Fatalf("extractAuthParams() code = %q, want %q", code, "token123")
	}

	if state != "" {
		t.Fatalf("extractAuthParams() state = %q, want empty", state)
	}
}

func TestExtractAuthParamsMissingCode(t *testing.T) {
	if _, _, err := extractAuthParams("https://app-api.pixiv.net/web/v1/users/auth/pixiv/callback?state=abc"); err == nil {
		t.Fatal("extractAuthParams() returned nil error, want error")
	}
}

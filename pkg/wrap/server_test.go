package wrap

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateDestinationURL(t *testing.T) {
	validCases := []string{
		"https://thgenius.xyz",
		"https://github.com/GOG-777/wrap-link",
		"http://example.com/path?query=1#hash",
	}

	for _, u := range validCases {
		if _, err := ValidateDestinationURL(u, "wrap.thgenius.xyz"); err != nil {
			t.Errorf("expected valid url %q, got error: %v", u, err)
		}
	}

	blockedCases := []string{
		"",
		"javascript:alert(1)",
		"data:text/html,<script>alert(1)</script>",
		"file:///etc/passwd",
		"http://127.0.0.1:8080/admin",
		"http://localhost:3000",
		"http://169.254.169.254/latest/meta-data/",
		"http://10.0.0.1/internal",
		"http://192.168.1.1/router",
		"https://wrap.thgenius.xyz/loop",
		"https://admin:secret@example.com/",
		"https://example.com/bad\r\nHeader: injected",
	}

	for _, u := range blockedCases {
		if _, err := ValidateDestinationURL(u, "wrap.thgenius.xyz"); err == nil {
			t.Errorf("expected url %q to be blocked, but it passed validation", u)
		}
	}
}

func TestStatelessTokenTampering(t *testing.T) {
	secret := []byte("unit-test-secret-key")
	target := "https://portfolio.thgenius.xyz"

	token, err := EncodeStateless(target, 0, secret)
	if err != nil {
		t.Fatalf("encode failed: %v", err)
	}

	gotURL, _, err := DecodeStateless(token, secret)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if gotURL != target {
		t.Fatalf("expected %q, got %q", target, gotURL)
	}

	// Tamper with the signature bytes
	tampered := "A" + token[1:]
	if token[0] == 'A' {
		tampered = "B" + token[1:]
	}
	if _, _, err := DecodeStateless(tampered, secret); err == nil {
		t.Errorf("expected tampered token to be rejected")
	}
}

func TestShortenRedirectAndStatsFlow(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "links.json")
	srv := NewServer(tmpFile, "public")

	body := `{"url":"https://github.com/GOG-777","alias":"gog-gh","ttl":"24h"}`
	req := httptest.NewRequest(http.MethodPost, "https://wrap.thgenius.xyz/api/shorten", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "203.0.113.10:12345"

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp shortenResponse
	if err := json.NewDecoder(bytes.NewReader(rec.Body.Bytes())).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Code != "gog-gh" {
		t.Fatalf("expected alias gog-gh, got %q", resp.Code)
	}

	// Perform redirect
	redReq := httptest.NewRequest(http.MethodGet, "https://wrap.thgenius.xyz/gog-gh", nil)
	redReq.RemoteAddr = "198.51.100.42:54321"
	redRec := httptest.NewRecorder()
	srv.ServeHTTP(redRec, redReq)

	if redRec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected 307 redirect, got %d", redRec.Code)
	}
	if loc := redRec.Header().Get("Location"); loc != "https://github.com/GOG-777" {
		t.Fatalf("unexpected redirect location: %q", loc)
	}

	// Verify click count via stats endpoint
	statReq := httptest.NewRequest(http.MethodGet, "https://wrap.thgenius.xyz/api/stats?code=gog-gh", nil)
	statReq.RemoteAddr = "198.51.100.42:54321"
	statRec := httptest.NewRecorder()
	srv.ServeHTTP(statRec, statReq)

	if statRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from stats, got %d", statRec.Code)
	}

	var stats LinkRecord
	if err := json.NewDecoder(bytes.NewReader(statRec.Body.Bytes())).Decode(&stats); err != nil {
		t.Fatalf("decode stats failed: %v", err)
	}
	if stats.Clicks != 1 {
		t.Fatalf("expected 1 click, got %d", stats.Clicks)
	}
}

func TestEmbeddedStaticAssets(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "links.json")
	srv := NewServer(tmpFile, "/nonexistent-public-dir")

	paths := []string{"/", "/docs", "/styles.css", "/app.js", "/favicon.svg", "/robots.txt"}
	for _, p := range paths {
		req := httptest.NewRequest(http.MethodGet, "https://wrap.thgenius.xyz"+p, nil)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for embedded path %q, got %d", p, rec.Code)
		}
		if rec.Body.Len() == 0 {
			t.Fatalf("expected non-empty body for embedded path %q", p)
		}
	}
}

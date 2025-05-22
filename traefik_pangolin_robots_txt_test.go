package traefik_pangolin_robots_txt_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	plugin "github.com/hhftechnology/traefik-pangolin-robots-txt"
)

// TestCustomRules verifies that custom rules are properly appended to robots.txt
func TestCustomRules(t *testing.T) {
	cfg := plugin.CreateConfig()
	cfg.CustomRules = "\nUser-agent: *\nDisallow: /private/\n"

	ctx := context.Background()
	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {})

	handler, err := plugin.New(ctx, next, cfg, "robots-txt-plugin")
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost/robots.txt", nil)
	if err != nil {
		t.Fatal(err)
	}

	handler.ServeHTTP(recorder, req)

	wantedRes := "\n# The following content was added on the fly by the Pangolin Robots.txt Traefik plugin: " +
		"https://github.com/hhftechnology/traefik-pangolin-robots-txt\n" +
		cfg.CustomRules
	if !bytes.Equal([]byte(wantedRes), recorder.Body.Bytes()) {
		t.Errorf("got body %q, want %q", recorder.Body.Bytes(), wantedRes)
	}

	if recorder.Code != http.StatusOK {
		t.Errorf("got status code %d, want %d", recorder.Code, http.StatusOK)
	}
}

// TestOverwriteOption tests the overwrite functionality that replaces original content
func TestOverwriteOption(t *testing.T) {
	cfg := plugin.CreateConfig()
	cfg.CustomRules = "\nUser-agent: *\nDisallow: /admin/\n"
	cfg.Overwrite = true // This should remove the original content

	ctx := context.Background()
	
	// Mock backend that returns existing robots.txt content
	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusOK)
		_, _ = rw.Write([]byte("User-agent: *\nDisallow: /old-path/\n"))
	})

	handler, err := plugin.New(ctx, next, cfg, "robots-txt-plugin")
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost/robots.txt", nil)
	if err != nil {
		t.Fatal(err)
	}

	handler.ServeHTTP(recorder, req)

	// With overwrite=true, the original content should NOT be included
	wantedRes := "# The following content was added on the fly by the Pangolin Robots.txt Traefik plugin: " +
		"https://github.com/hhftechnology/traefik-pangolin-robots-txt\n" +
		cfg.CustomRules
	
	actualBody := recorder.Body.String()
	
	// Verify original content is not present
	if strings.Contains(actualBody, "/old-path/") {
		t.Errorf("overwrite=true should remove original content, but found '/old-path/' in: %s", actualBody)
	}
	
	// Verify our custom rules are present
	if !strings.Contains(actualBody, "/admin/") {
		t.Errorf("custom rules should be present, but '/admin/' not found in: %s", actualBody)
	}
	
	if !bytes.Equal([]byte(wantedRes), recorder.Body.Bytes()) {
		t.Errorf("got body %q, want %q", actualBody, wantedRes)
	}
}

// TestOverwriteWithoutOriginalContent tests overwrite when no original content exists
func TestOverwriteWithoutOriginalContent(t *testing.T) {
	cfg := plugin.CreateConfig()
	cfg.CustomRules = "\nUser-agent: *\nDisallow: /secure/\n"
	cfg.Overwrite = true

	ctx := context.Background()
	
	// Mock backend that returns 404 (no robots.txt exists)
	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusNotFound)
	})

	handler, err := plugin.New(ctx, next, cfg, "robots-txt-plugin")
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost/robots.txt", nil)
	if err != nil {
		t.Fatal(err)
	}

	handler.ServeHTTP(recorder, req)

	// Should work the same whether overwrite is true or false when no original content
	expectedContent := "# The following content was added on the fly by the Pangolin Robots.txt Traefik plugin: " +
		"https://github.com/hhftechnology/traefik-pangolin-robots-txt\n" +
		cfg.CustomRules
	
	if recorder.Body.String() != expectedContent {
		t.Errorf("got body %q, want %q", recorder.Body.String(), expectedContent)
	}
}

// TestAiRobotsTxtWithMockServer tests fetching AI robots.txt from a custom URL
func TestAiRobotsTxtWithMockServer(t *testing.T) {
	// Create a mock server that serves robots.txt content
	mockContent := "User-agent: GPTBot\nDisallow: /\n\nUser-agent: ChatGPT-User\nDisallow: /\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(mockContent))
	}))
	defer server.Close()

	cfg := plugin.CreateConfig()
	cfg.AiRobotsTxt = true
	cfg.AiRobotsTxtURL = server.URL // Use our mock server instead of GitHub
	cfg.CustomRules = "\nUser-agent: *\nDisallow: /api/\n"

	ctx := context.Background()
	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {})

	handler, err := plugin.New(ctx, next, cfg, "robots-txt-plugin")
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost/robots.txt", nil)
	if err != nil {
		t.Fatal(err)
	}

	handler.ServeHTTP(recorder, req)

	body := recorder.Body.String()
	
	// Should contain both the fetched AI content and custom rules
	if !strings.Contains(body, "GPTBot") {
		t.Errorf("should contain fetched AI robots content, but 'GPTBot' not found in: %s", body)
	}
	
	if !strings.Contains(body, "/api/") {
		t.Errorf("should contain custom rules, but '/api/' not found in: %s", body)
	}

	if recorder.Code != http.StatusOK {
		t.Errorf("got status code %d, want %d", recorder.Code, http.StatusOK)
	}
}

// TestAiRobotsTxtFromFile tests reading AI robots.txt content from a local file
func TestAiRobotsTxtFromFile(t *testing.T) {
	// Create a temporary file with robots.txt content
	tempDir := t.TempDir()
	robotsFile := filepath.Join(tempDir, "ai-robots.txt")
	
	fileContent := "User-agent: GoogleBot\nDisallow: /private/\n\nUser-agent: BingBot\nDisallow: /temp/\n"
	if err := os.WriteFile(robotsFile, []byte(fileContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := plugin.CreateConfig()
	cfg.AiRobotsTxt = true
	cfg.AiRobotsTxtPath = robotsFile // Use local file instead of URL
	cfg.CustomRules = "\nUser-agent: *\nAllow: /public/\n"

	ctx := context.Background()
	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {})

	handler, err := plugin.New(ctx, next, cfg, "robots-txt-plugin")
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost/robots.txt", nil)
	if err != nil {
		t.Fatal(err)
	}

	handler.ServeHTTP(recorder, req)

	body := recorder.Body.String()
	
	// Should contain content from the file
	if !strings.Contains(body, "GoogleBot") {
		t.Errorf("should contain file content, but 'GoogleBot' not found in: %s", body)
	}
	
	if !strings.Contains(body, "/public/") {
		t.Errorf("should contain custom rules, but '/public/' not found in: %s", body)
	}
}

// TestCachingBehavior tests that the caching mechanism works correctly
func TestCachingBehavior(t *testing.T) {
	callCount := 0
	mockContent := "User-agent: TestBot\nDisallow: /cached/\n"
	
	// Mock server that counts requests
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(mockContent))
	}))
	defer server.Close()

	cfg := plugin.CreateConfig()
	cfg.AiRobotsTxt = true
	cfg.AiRobotsTxtURL = server.URL
	cfg.CacheTTL = 2 // 2 seconds for quick testing
	cfg.EnableMetrics = true

	ctx := context.Background()
	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {})

	handler, err := plugin.New(ctx, next, cfg, "robots-txt-plugin")
	if err != nil {
		t.Fatal(err)
	}

	// First request should hit the server
	req1, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost/robots.txt", nil)
	recorder1 := httptest.NewRecorder()
	handler.ServeHTTP(recorder1, req1)

	if callCount != 1 {
		t.Errorf("expected 1 call to server, got %d", callCount)
	}

	// Second request should use cache
	req2, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost/robots.txt", nil)
	recorder2 := httptest.NewRecorder()
	handler.ServeHTTP(recorder2, req2)

	if callCount != 1 {
		t.Errorf("expected still 1 call to server (cached), got %d", callCount)
	}

	// Wait for cache to expire and make another request
	time.Sleep(3 * time.Second)
	req3, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost/robots.txt", nil)
	recorder3 := httptest.NewRecorder()
	handler.ServeHTTP(recorder3, req3)

	if callCount != 2 {
		t.Errorf("expected 2 calls to server (cache expired), got %d", callCount)
	}
}

// TestFallbackContent tests fallback mechanism when external source fails
func TestFallbackContent(t *testing.T) {
	// Mock server that always returns an error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	cfg := plugin.CreateConfig()
	cfg.AiRobotsTxt = true
	cfg.AiRobotsTxtURL = server.URL
	cfg.FallbackContent = "User-agent: *\nDisallow: /fallback/\n"
	cfg.CustomRules = "\nUser-agent: *\nDisallow: /custom/\n"
	cfg.MaxRetries = 1 // Reduce retries for faster testing

	ctx := context.Background()
	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {})

	handler, err := plugin.New(ctx, next, cfg, "robots-txt-plugin")
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost/robots.txt", nil)
	if err != nil {
		t.Fatal(err)
	}

	handler.ServeHTTP(recorder, req)

	body := recorder.Body.String()
	
	// Should contain fallback content since external source failed
	if !strings.Contains(body, "/fallback/") {
		t.Errorf("should contain fallback content, but '/fallback/' not found in: %s", body)
	}
	
	// Should still contain custom rules
	if !strings.Contains(body, "/custom/") {
		t.Errorf("should contain custom rules, but '/custom/' not found in: %s", body)
	}
}

// TestInvalidConfiguration tests that invalid configurations are rejected
func TestInvalidConfiguration(t *testing.T) {
	testCases := []struct {
		name   string
		config func() *plugin.Config
	}{
		{
			name: "No options enabled",
			config: func() *plugin.Config {
				cfg := plugin.CreateConfig()
				cfg.CustomRules = ""
				cfg.AiRobotsTxt = false
				return cfg
			},
		},
		{
			name: "Invalid file path",
			config: func() *plugin.Config {
				cfg := plugin.CreateConfig()
				cfg.AiRobotsTxt = true
				cfg.AiRobotsTxtPath = "relative/path/robots.txt" // Should be absolute
				return cfg
			},
		},
	}

	ctx := context.Background()
	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {})

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := plugin.New(ctx, next, tc.config(), "robots-txt-plugin")
			if err == nil {
				t.Errorf("expected error for test case %s, but got none", tc.name)
			}
		})
	}
}

// TestNonRobotsTxtRequests ensures non-robots.txt requests pass through unchanged
func TestNonRobotsTxtRequests(t *testing.T) {
	cfg := plugin.CreateConfig()
	cfg.CustomRules = "\nUser-agent: *\nDisallow: /private/\n"

	ctx := context.Background()
	
	// Mock backend that should be called for non-robots.txt requests
	backendCalled := false
	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		backendCalled = true
		rw.WriteHeader(http.StatusOK)
		_, _ = rw.Write([]byte("This is not robots.txt"))
	})

	handler, err := plugin.New(ctx, next, cfg, "robots-txt-plugin")
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost/index.html", nil)
	if err != nil {
		t.Fatal(err)
	}

	handler.ServeHTTP(recorder, req)

	if !backendCalled {
		t.Error("backend should be called for non-robots.txt requests")
	}

	if recorder.Body.String() != "This is not robots.txt" {
		t.Errorf("non-robots.txt requests should pass through unchanged")
	}
}

// TestAiRobotsTxt tests the original functionality with real GitHub URL
func TestAiRobotsTxt(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test that makes external HTTP request in short mode")
	}

	cfg := plugin.CreateConfig()
	cfg.AiRobotsTxt = true
	cfg.RequestTimeout = 30 // Longer timeout for real request

	ctx := context.Background()
	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {})

	handler, err := plugin.New(ctx, next, cfg, "robots-txt-plugin")
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost/robots.txt", nil)
	if err != nil {
		t.Fatal(err)
	}

	handler.ServeHTTP(recorder, req)

	body := recorder.Body.String()
	
	// Should contain some kind of robots.txt content (exact content may change)
	if !strings.Contains(body, "User-agent:") && !strings.Contains(body, "Disallow:") {
		t.Errorf("should contain robots.txt patterns, got: %s", body)
	}

	if recorder.Code != http.StatusOK {
		t.Errorf("got status code %d, want %d", recorder.Code, http.StatusOK)
	}
}

// TestNoOption tests the original validation error scenario
func TestNoOption(t *testing.T) {
	cfg := plugin.CreateConfig()
	cfg.CustomRules = ""
	cfg.AiRobotsTxt = false

	ctx := context.Background()
	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {})

	_, err := plugin.New(ctx, next, cfg, "robots-txt-plugin")
	if err == nil {
		t.Fatal(errors.New("an error should be raised"))
	} else {
		errMsg := "set customRules or set aiRobotsTxt to true"
		if err.Error() != errMsg {
			t.Errorf("got err message %s, want %s", err.Error(), errMsg)
		}
	}
}
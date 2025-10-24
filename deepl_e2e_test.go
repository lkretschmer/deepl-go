//go:build e2e
// +build e2e

// End-to-End (E2E) tests for the DeepL Go client library (See Issue #4).
//
// Note: In some IDEs, "No packages found for open file" warnings may appear.
//       This is expected due to the build tag "e2e".
//
// These tests use the DeepL Mock server (https://github.com/DeepLcom/deepl-mock)
// to simulate real DeepL API interactions without requiring actual API credentials.
//
// Requirements:
//   - Docker
//   - Docker Compose
//
// How to run the tests (both E2E and usual unit tests):
//
//	$ docker compose up deepl-mock -d
//	$ docker compose run --rm deepl-test
//	$ docker compose down --remove-orphans
//
// Local Development Alternative (spawn only mock server and run tests manually):
//
//	$ docker compose up deepl-mock -d
//	$ go test --tags=e2e -v ./...
//	$ docker compose down --remove-orphans

package deepl_test

import (
	"github.com/lkretschmer/deepl-go"

	"context"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

const (
	// Dummy API key (API key is ignored by mock server)
	mockAPIKey = "mock-api-key"
	// Test timeouts
	testTimeout = 30 * time.Second
	// User agent for E2E tests
	testUserAgent = "deepl-go-e2e-test"
	// Default mock server URL for local development
	// In Docker Compose, uses DEEPL_SERVER_URL=http://deepl-mock:3000
	defaultMockServerURL = "http://localhost:3000"
	// Default mock server proxy URL for local development
	// In Docker Compose, uses DEEPL_PROXY_URL=http://deepl-mock:3001
	defaultMockServerProxyURL = "http://localhost:3001"
)

// getMockServerURL returns the mock server URL from environment variable or default.
// In Docker Compose environment, DEEPL_SERVER_URL should be set to http://deepl-mock:3000.
// The default localhost URL is for local development when running mock server directly.
func getMockServerURL() string {
	if url := os.Getenv("DEEPL_SERVER_URL"); url != "" {
		return url
	}
	return defaultMockServerURL
}

// getMockServerProxyURL returns the mock server proxy URL from environment variable or default.
// In Docker Compose environment, DEEPL_PROXY_URL should be set to http://deepl-mock:3001.
// The default localhost URL is for local development when running mock server directly.
func getMockServerProxyURL() string {
	if url := os.Getenv("DEEPL_PROXY_URL"); url != "" {
		return url
	}
	return defaultMockServerProxyURL
}

// createTestClient creates a DeepL client configured for E2E testing
func createTestClient(serverURL string) *deepl.Client {
	return deepl.NewClient(mockAPIKey,
		deepl.WithBaseURL(serverURL),
		deepl.WithUserAgent(testUserAgent),
	)
}

// waitForMockServer waits for the mock server to be ready
func waitForMockServer(t *testing.T, serverURL string) {
	t.Helper()

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			t.Fatal("Mock server did not become ready in time")
		case <-ticker.C:
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, serverURL+"/v2/usage?auth_key=smoke_test", nil)
			if err != nil {
				continue
			}

			req.Header.Set("User-Agent", testUserAgent)

			resp, err := client.Do(req)
			if err == nil && resp.StatusCode == http.StatusOK {
				resp.Body.Close()
				return
			}
			if resp != nil {
				resp.Body.Close()
			}
		}
	}
}

func TestE2E_MockServerHealth(t *testing.T) {
	serverURL := getMockServerURL()
	waitForMockServer(t, serverURL)

	// Create HTTP client for direct API calls
	client := &http.Client{Timeout: 10 * time.Second}

	// Test health endpoint
	req, err := http.NewRequest(http.MethodGet, serverURL+"/v2/usage?auth_key=smoke_test", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	req.Header.Set("User-Agent", testUserAgent)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status code %d, got %d", http.StatusOK, resp.StatusCode)
	}
}

func TestE2E_DeepLClient_GetUsage(t *testing.T) {
	serverURL := getMockServerURL()
	waitForMockServer(t, serverURL)

	// Create DeepL client with mock server URL
	client := createTestClient(serverURL)

	// Test GetUsage
	usage, err := client.GetUsage()
	if err != nil {
		t.Fatalf("GetUsage should succeed with mock server, got error: %v", err)
	}

	if usage == nil {
		t.Fatal("Usage response should not be nil")
	}

	// Mock server should return some usage data
	t.Logf("Usage response: CharacterCount=%d, CharacterLimit=%d",
		usage.CharacterCount, usage.CharacterLimit)
}

func TestE2E_DeepLClient_TranslateText(t *testing.T) {
	serverURL := getMockServerURL()
	waitForMockServer(t, serverURL)

	// Create DeepL client with mock server URL
	client := createTestClient(serverURL)

	// Test TranslateText
	text := "Hello, world!"
	targetLang := "JA"

	translation, err := client.TranslateText(text, targetLang)
	if err != nil {
		t.Fatalf("TranslateText should succeed with mock server, got error: %v", err)
	}

	if translation == nil {
		t.Fatal("Should return a translation")
	}
	if translation.Text == "" {
		t.Error("Translation text should not be empty")
	}
	if translation.DetectedSourceLanguage == "" {
		t.Error("Should detect source language")
	}

	t.Logf("Translation: '%s' -> '%s'", text, translation.Text)
}

func TestE2E_DeepLClient_GetSupportedLanguages(t *testing.T) {
	serverURL := getMockServerURL()
	waitForMockServer(t, serverURL)

	// Create DeepL client with mock server URL
	client := createTestClient(serverURL)

	// Test GetSourceLanguages
	sourceLangs, err := client.GetSourceLanguages()
	if err != nil {
		t.Fatalf("GetSourceLanguages should succeed, got error: %v", err)
	}
	if len(sourceLangs) == 0 {
		t.Error("Should return supported source languages")
	}

	// Test GetTargetLanguages
	targetLangs, err := client.GetTargetLanguages()
	if err != nil {
		t.Fatalf("GetTargetLanguages should succeed, got error: %v", err)
	}
	if len(targetLangs) == 0 {
		t.Error("Should return supported target languages")
	}

	t.Logf("Found %d source languages and %d target languages",
		len(sourceLangs), len(targetLangs))
}

func TestE2E_DeepLClient_ErrorHandling(t *testing.T) {
	serverURL := getMockServerURL()
	waitForMockServer(t, serverURL)

	// Test with various invalid scenarios
	testCases := []struct {
		name   string
		apiKey string
	}{
		{"empty key", ""},
		{"invalid literal", "invalid"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client := deepl.NewClient(tc.apiKey,
				deepl.WithBaseURL(serverURL),
				deepl.WithUserAgent(testUserAgent),
			)

			// Test with invalid API key - may or may not return error depending on mock server behavior
			usage, err := client.GetUsage()

			// Log the result regardless
			if err != nil {
				t.Logf("API key '%s' returned error: %v", tc.apiKey, err)
				if !strings.Contains(err.Error(), "HTTP") {
					t.Errorf("Error should contain HTTP status information, got: %v", err)
				}
			} else {
				t.Logf("API key '%s' succeeded with usage: %+v", tc.apiKey, usage)
				// Mock server may accept invalid keys for testing purposes
				if usage == nil {
					t.Error("Usage response should not be nil")
				}
			}
		})
	}
}

func TestE2E_DeepLClient_WithProxy(t *testing.T) {
	serverURL := getMockServerURL()
	proxyURL := getMockServerProxyURL()
	waitForMockServer(t, serverURL)

	// Parse proxy URL
	parsedProxyURL, err := url.Parse(proxyURL)
	if err != nil {
		t.Fatalf("Failed to parse proxy URL: %v", err)
	}

	// Create client configured to use proxy
	client := deepl.NewClient(mockAPIKey,
		deepl.WithBaseURL(serverURL),
		deepl.WithUserAgent(testUserAgent),
		deepl.WithProxy(*parsedProxyURL),
	)

	// Test that requests work through the proxy
	// The mock server's proxy at port 3001 will forward requests to port 3000
	usage, err := client.GetUsage()
	if err != nil {
		t.Fatalf("GetUsage through proxy should succeed, got error: %v", err)
	}

	if usage == nil {
		t.Fatal("Usage response should not be nil")
	}

	t.Logf("Successfully tested proxy: requests routed through %s", proxyURL)
	t.Logf("Usage response: CharacterCount=%d, CharacterLimit=%d",
		usage.CharacterCount, usage.CharacterLimit)
}

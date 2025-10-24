package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// APISpecFetcher tests
// ----------------------------------------------------------------------------

// TestAPISpecFetcher tests APISpecFetcher functionality including fetching,
// caching, and error handling.
func TestAPISpecFetcher(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T) (*APISpecFetcher, *httptest.Server, string)
		expectError bool
		errorMsg    string
		validate    func(t *testing.T, spec *OpenAPISpec, cachePath string)
	}{
		{
			name: "successful fetch",
			setup: func(t *testing.T) (*APISpecFetcher, *httptest.Server, string) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method != "GET" {
						t.Errorf("Expected GET request, got %s", r.Method)
					}
					if r.Header.Get("User-Agent") != "deepl-go-coverage-analyzer/1.0" {
						t.Errorf("Expected User-Agent header, got %s", r.Header.Get("User-Agent"))
					}
					w.Header().Set("Content-Type", "application/yaml")
					if _, err := fmt.Fprint(w, `
info:
  title: DeepL API
  version: 1.0.0
paths:
  /v2/translate:
    post:
      operationId: translateText
      summary: Translate text
`); err != nil {
						t.Fatal(err)
					}
				}))
				tempDir := t.TempDir()
				cachePath := filepath.Join(tempDir, "test_spec.yaml")
				fetcher := NewAPISpecFetcher()
				fetcher.HTTPClient = server.Client()
				fetcher.URL = server.URL
				fetcher.CachePath = cachePath
				fetcher.Timeout = 5 * time.Second
				fetcher.Logger = func(format string, args ...interface{}) {}
				return fetcher, server, cachePath
			},
			expectError: false,
			validate: func(t *testing.T, spec *OpenAPISpec, cachePath string) {
				if spec.Info.Title != "DeepL API" {
					t.Errorf("Expected title 'DeepL API', got %s", spec.Info.Title)
				}
				if spec.Info.Version != "1.0.0" {
					t.Errorf("Expected version '1.0.0', got %s", spec.Info.Version)
				}
				if _, err := os.Stat(cachePath); os.IsNotExist(err) {
					t.Error("Expected cache file to be created")
				}
			},
		},
		{
			name: "load cached spec",
			setup: func(t *testing.T) (*APISpecFetcher, *httptest.Server, string) {
				tempDir := t.TempDir()
				cachePath := filepath.Join(tempDir, "cached_spec.yaml")
				yamlContent := `
info:
  title: Cached DeepL API
  version: 2.0.0
paths: {}
`
				err := os.WriteFile(cachePath, []byte(yamlContent), 0644)
				if err != nil {
					t.Fatal(err)
				}
				fetcher := &APISpecFetcher{
					CachePath: cachePath,
					Logger:    func(format string, args ...interface{}) {},
				}
				return fetcher, nil, cachePath
			},
			expectError: false,
			validate: func(t *testing.T, spec *OpenAPISpec, cachePath string) {
				if spec.Info.Title != "Cached DeepL API" {
					t.Errorf("Expected title 'Cached DeepL API', got %s", spec.Info.Title)
				}
			},
		},
		{
			name: "load cached spec no file",
			setup: func(t *testing.T) (*APISpecFetcher, *httptest.Server, string) {
				tempDir := t.TempDir()
				cachePath := filepath.Join(tempDir, "nonexistent.yaml")
				fetcher := &APISpecFetcher{
					CachePath: cachePath,
					Logger:    func(format string, args ...interface{}) {},
				}
				return fetcher, nil, cachePath
			},
			expectError: true,
		},
		{
			name: "load cached spec old file",
			setup: func(t *testing.T) (*APISpecFetcher, *httptest.Server, string) {
				tempDir := t.TempDir()
				cachePath := filepath.Join(tempDir, "old_spec.yaml")
				yamlContent := `info:
  title: Old API
  version: 1.0.0
paths: {}
`
				err := os.WriteFile(cachePath, []byte(yamlContent), 0644)
				if err != nil {
					t.Fatal(err)
				}
				oldTime := time.Now().Add(-2 * time.Hour)
				err = os.Chtimes(cachePath, oldTime, oldTime)
				if err != nil {
					t.Fatal(err)
				}
				fetcher := &APISpecFetcher{
					CachePath: cachePath,
					Logger:    func(format string, args ...interface{}) {},
				}
				return fetcher, nil, cachePath
			},
			expectError: true,
			errorMsg:    "too old",
		},
		{
			name: "fetch HTTP error",
			setup: func(t *testing.T) (*APISpecFetcher, *httptest.Server, string) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusNotFound)
				}))
				tempDir := t.TempDir()
				cachePath := filepath.Join(tempDir, "test_spec.yaml")
				fetcher := &APISpecFetcher{
					HTTPClient: server.Client(),
					URL:        server.URL,
					CachePath:  cachePath,
					Timeout:    5 * time.Second,
					Logger:     func(format string, args ...interface{}) {},
				}
				return fetcher, server, cachePath
			},
			expectError: true,
			errorMsg:    "HTTP error",
		},
		{
			name: "fetch invalid YAML",
			setup: func(t *testing.T) (*APISpecFetcher, *httptest.Server, string) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/yaml")
					if _, err := fmt.Fprint(w, "invalid: yaml: content: [unclosed"); err != nil {
						t.Fatal(err)
					}
				}))
				tempDir := t.TempDir()
				cachePath := filepath.Join(tempDir, "test_spec.yaml")
				fetcher := &APISpecFetcher{
					HTTPClient: server.Client(),
					URL:        server.URL,
					CachePath:  cachePath,
					Timeout:    5 * time.Second,
					Logger:     func(format string, args ...interface{}) {},
				}
				return fetcher, server, cachePath
			},
			expectError: true,
			errorMsg:    "failed to parse",
		},
		{
			name: "cache spec",
			setup: func(t *testing.T) (*APISpecFetcher, *httptest.Server, string) {
				tempDir := t.TempDir()
				cachePath := filepath.Join(tempDir, "test_cache.yaml")
				fetcher := &APISpecFetcher{
					CachePath: cachePath,
					Logger:    func(format string, args ...interface{}) {},
				}
				return fetcher, nil, cachePath
			},
			expectError: false,
			validate: func(t *testing.T, spec *OpenAPISpec, cachePath string) {
				yamlContent := []byte("test: content")
				fetcher := &APISpecFetcher{CachePath: cachePath, Logger: func(format string, args ...interface{}) {}}
				err := fetcher.cacheSpec(yamlContent)
				if err != nil {
					t.Fatalf("Expected no error caching spec, got %v", err)
				}
				cachedContent, err := os.ReadFile(cachePath)
				if err != nil {
					t.Fatal(err)
				}
				if string(cachedContent) != string(yamlContent) {
					t.Errorf("Cached content doesn't match original")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fetcher, server, cachePath := tt.setup(t)
			if server != nil {
				defer server.Close()
			}

			var spec *OpenAPISpec
			var err error

			switch tt.name {
			case "load cached spec", "load cached spec no file", "load cached spec old file":
				spec, err = fetcher.loadCachedSpec()
			case "cache spec":
				yamlContent := []byte("test: content")
				err = fetcher.cacheSpec(yamlContent)
				if err == nil {
					tt.validate(t, nil, cachePath)
				}
			default:
				spec, err = fetcher.Fetch()
			}

			if tt.expectError {
				if err == nil || !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error containing %q, got %v", tt.errorMsg, err)
				}
			} else {
				if err != nil {
					t.Fatalf("Expected no error, got %v", err)
				}
				if tt.validate != nil && spec != nil {
					tt.validate(t, spec, cachePath)
				}
			}
		})
	}
}

// OpenAPI parsing tests
// ----------------------------------------------------------------------------

func Test_parseOpenAPISpec(t *testing.T) {
	tests := []struct {
		name        string
		yamlContent string
		expectError bool
		errorMsg    string
		validate    func(t *testing.T, spec *OpenAPISpec)
	}{
		{
			name: "valid YAML",
			yamlContent: `
info:
  title: Test API
  version: 1.0.0
paths:
  /test:
    get:
      operationId: testOp
      summary: Test operation
`,
			expectError: false,
			validate: func(t *testing.T, spec *OpenAPISpec) {
				if spec.Info.Title != "Test API" {
					t.Errorf("Expected title 'Test API', got %s", spec.Info.Title)
				}
				if len(spec.Paths) != 1 {
					t.Errorf("Expected 1 path, got %d", len(spec.Paths))
				}
			},
		},
		{
			name:        "empty content",
			yamlContent: "",
			expectError: true,
			errorMsg:    "empty YAML content",
		},
		{
			name:        "invalid YAML",
			yamlContent: `invalid: yaml: content: [unclosed`,
			expectError: true,
			errorMsg:    "failed to parse YAML",
		},
		{
			name: "missing title",
			yamlContent: `
info:
  version: 1.0.0
paths: {}
`,
			expectError: true,
			errorMsg:    "missing required field",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, err := parseOpenAPISpec([]byte(tt.yamlContent))
			if tt.expectError {
				if err == nil || !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error containing %q, got %v", tt.errorMsg, err)
				}
			} else {
				if err != nil {
					t.Fatalf("Expected no error, got %v", err)
				}
				if tt.validate != nil {
					tt.validate(t, spec)
				}
			}
		})
	}
}

// Mock types for testing
// ----------------------------------------------------------------------------

// MockFileWalker for testing
type MockFileWalker struct {
	WalkFunc func(root string, walkFn filepath.WalkFunc) error
}

func (m *MockFileWalker) Walk(root string, walkFn filepath.WalkFunc) error {
	if m.WalkFunc != nil {
		return m.WalkFunc(root, walkFn)
	}
	return nil
}

// MockLogger for testing
type MockLogger struct {
	Logs []string
}

func (m *MockLogger) Log(format string, args ...interface{}) {
	log := fmt.Sprintf(format, args...)
	m.Logs = append(m.Logs, log)
}

// GoSourceAnalyzer tests
// ----------------------------------------------------------------------------

func TestGoSourceAnalyzer_Analyze(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T) (*GoSourceAnalyzer, string, *MockLogger)
		expectError bool
		errorMsg    string
		validate    func(t *testing.T, methods []GoMethod, logger *MockLogger)
	}{
		{
			name: "normal analysis",
			setup: func(t *testing.T) (*GoSourceAnalyzer, string, *MockLogger) {
				tempDir := t.TempDir()
				testFile := filepath.Join(tempDir, "client.go")
				testContent := `
package main

type Client struct{}

func (c *Client) TranslateText(text string) (string, error) {
	return "translated", nil
}

func (c *Client) GetLanguages() ([]string, error) {
	return []string{"en", "de"}, nil
}
`
				err := os.WriteFile(testFile, []byte(testContent), 0644)
				if err != nil {
					t.Fatal(err)
				}
				mockLogger := &MockLogger{}
				analyzer := &GoSourceAnalyzer{
					FileWalker: &OSFileWalker{},
					Logger:     mockLogger.Log,
				}
				return analyzer, tempDir, mockLogger
			},
			expectError: false,
			validate: func(t *testing.T, methods []GoMethod, logger *MockLogger) {
				if len(methods) != 2 {
					t.Errorf("Expected 2 methods, got %d", len(methods))
				}
				foundTranslate := false
				foundGetLanguages := false
				for _, method := range methods {
					if method.Name == "TranslateText" {
						foundTranslate = true
						if method.Receiver != "*Client" {
							t.Errorf("Expected receiver '*Client', got %s", method.Receiver)
						}
					}
					if method.Name == "GetLanguages" {
						foundGetLanguages = true
					}
				}
				if !foundTranslate {
					t.Error("TranslateText method not found")
				}
				if !foundGetLanguages {
					t.Error("GetLanguages method not found")
				}
			},
		},
		{
			name: "walk error",
			setup: func(t *testing.T) (*GoSourceAnalyzer, string, *MockLogger) {
				mockLogger := &MockLogger{}
				mockWalker := &MockFileWalker{
					WalkFunc: func(root string, walkFn filepath.WalkFunc) error {
						return fmt.Errorf("walk error")
					},
				}
				analyzer := &GoSourceAnalyzer{
					FileWalker: mockWalker,
					Logger:     mockLogger.Log,
				}
				return analyzer, "/tmp", mockLogger
			},
			expectError: true,
			errorMsg:    "walk error",
		},
		{
			name: "file filtering",
			setup: func(t *testing.T) (*GoSourceAnalyzer, string, *MockLogger) {
				// Create test files that should be filtered out (test files, generated files, testdata)
				tempDir := t.TempDir()
				// Create files that should be skipped
				testFile := filepath.Join(tempDir, "client_test.go")
				err := os.WriteFile(testFile, []byte("func TestSomething() {}"), 0644)
				if err != nil {
					t.Fatal(err)
				}
				genFile := filepath.Join(tempDir, "gen_api_coverage.go")
				err = os.WriteFile(genFile, []byte("func main() {}"), 0644)
				if err != nil {
					t.Fatal(err)
				}
				testdataDir := filepath.Join(tempDir, "testdata")
				err = os.Mkdir(testdataDir, 0755)
				if err != nil {
					t.Fatal(err)
				}
				testdataFile := filepath.Join(testdataDir, "data.go")
				err = os.WriteFile(testdataFile, []byte("func Data() {}"), 0644)
				if err != nil {
					t.Fatal(err)
				}
				mockLogger := &MockLogger{}
				analyzer := &GoSourceAnalyzer{
					FileWalker: &OSFileWalker{},
					Logger:     mockLogger.Log,
				}
				return analyzer, tempDir, mockLogger
			},
			expectError: false,
			validate: func(t *testing.T, methods []GoMethod, logger *MockLogger) {
				if len(methods) != 0 {
					t.Errorf("Expected 0 methods (all filtered), got %d", len(methods))
				}
			},
		},
		{
			name: "parse error",
			setup: func(t *testing.T) (*GoSourceAnalyzer, string, *MockLogger) {
				tempDir := t.TempDir()
				invalidFile := filepath.Join(tempDir, "invalid.go")
				err := os.WriteFile(invalidFile, []byte("invalid go syntax {{{{"), 0644)
				if err != nil {
					t.Fatal(err)
				}
				mockLogger := &MockLogger{}
				analyzer := &GoSourceAnalyzer{
					FileWalker: &OSFileWalker{},
					Logger:     mockLogger.Log,
				}
				return analyzer, tempDir, mockLogger
			},
			expectError: false,
			validate: func(t *testing.T, methods []GoMethod, logger *MockLogger) {
				if len(methods) != 0 {
					t.Errorf("Expected 0 methods due to parse error, got %d", len(methods))
				}
				hasWarning := false
				for _, log := range logger.Logs {
					if strings.Contains(log, "Warning") && strings.Contains(log, "failed to parse") {
						hasWarning = true
						break
					}
				}
				if !hasWarning {
					t.Error("Expected parse error warning to be logged")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analyzer, root, logger := tt.setup(t)
			methods, err := analyzer.Analyze(root)
			if tt.expectError {
				if err == nil || !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error containing %q, got %v", tt.errorMsg, err)
				}
			} else {
				if err != nil {
					t.Fatalf("Expected no error, got %v", err)
				}
				if tt.validate != nil {
					tt.validate(t, methods, logger)
				}
			}
		})
	}
}

// AST analysis tests
// ----------------------------------------------------------------------------

func Test_parseGoFile(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T) string
		expectError bool
		validate    func(t *testing.T, methods []GoMethod)
	}{
		{
			name: "valid Go file",
			setup: func(t *testing.T) string {
				// Create a test Go file with client methods and a non-client function
				tempDir := t.TempDir()
				testFile := filepath.Join(tempDir, "test_client.go")
				testContent := `
package main

type Client struct{}

func (c *Client) TranslateText(text string) (string, error) {
	return "translated", nil
}

func (c *Client) GetLanguages() ([]string, error) {
	return []string{"en", "de"}, nil
}

func nonClientMethod() {
	// This should not be detected
}
`
				err := os.WriteFile(testFile, []byte(testContent), 0644)
				if err != nil {
					t.Fatal(err)
				}
				return testFile
			},
			expectError: false,
			validate: func(t *testing.T, methods []GoMethod) {
				if len(methods) != 2 {
					t.Errorf("Expected 2 methods, got %d", len(methods))
					for i, m := range methods {
						t.Logf("Method %d: %s", i, m.Name)
					}
				}
				foundTranslate := false
				foundGetLanguages := false
				for _, method := range methods {
					if method.Name == "TranslateText" {
						foundTranslate = true
						if method.Receiver != "*Client" {
							t.Errorf("Expected receiver '*Client', got %s", method.Receiver)
						}
						if len(method.Parameters) != 1 || method.Parameters[0] != "text string" {
							t.Errorf("Expected parameters ['text string'], got %v", method.Parameters)
						}
						if len(method.ReturnTypes) != 2 || method.ReturnTypes[0] != "string" || method.ReturnTypes[1] != "error" {
							t.Errorf("Expected return types ['string', 'error'], got %v", method.ReturnTypes)
						}
					}
					if method.Name == "GetLanguages" {
						foundGetLanguages = true
					}
				}
				if !foundTranslate {
					t.Error("TranslateText method not found")
				}
				if !foundGetLanguages {
					t.Error("GetLanguages method not found")
				}
			},
		},
		{
			name: "invalid syntax",
			setup: func(t *testing.T) string {
				tempDir := t.TempDir()
				invalidFile := filepath.Join(tempDir, "invalid.go")
				err := os.WriteFile(invalidFile, []byte("invalid syntax {{{{"), 0644)
				if err != nil {
					t.Fatal(err)
				}
				return invalidFile
			},
			expectError: true,
		},
		{
			name: "no methods",
			setup: func(t *testing.T) string {
				tempDir := t.TempDir()
				noMethodFile := filepath.Join(tempDir, "no_methods.go")
				err := os.WriteFile(noMethodFile, []byte("package main\n\nconst x = 1"), 0644)
				if err != nil {
					t.Fatal(err)
				}
				return noMethodFile
			},
			expectError: false,
			validate: func(t *testing.T, methods []GoMethod) {
				if len(methods) != 0 {
					t.Errorf("Expected 0 methods, got %d", len(methods))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filePath := tt.setup(t)
			methods, err := parseGoFile(filePath)
			if tt.expectError {
				if err == nil {
					t.Error("Expected parse error")
				}
			} else {
				if err != nil {
					t.Fatalf("Expected no error, got %v", err)
				}
				if tt.validate != nil {
					tt.validate(t, methods)
				}
			}
		})
	}
}

func Test_extractMethodInfo(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		funcName string
		expected GoMethod
	}{
		{
			name: "method with receiver",
			src: `
package main

type Client struct{}

func (c *Client) TranslateText(ctx context.Context, text string, opts *TranslateOptions) (*TranslateResponse, error) {
	return &TranslateResponse{Text: "translated"}, nil
}
`,
			funcName: "TranslateText",
			expected: GoMethod{
				Name:        "TranslateText",
				Receiver:    "*Client",
				Parameters:  []string{"ctx context.Context", "text string", "opts *TranslateOptions"},
				ReturnTypes: []string{"*TranslateResponse", "error"},
				FileName:    "test.go",
				LineNumber:  6,
			},
		},
		{
			name: "global function",
			src: `
package main

func globalFunction() {
}
`,
			funcName: "globalFunction",
			expected: GoMethod{
				Name:        "globalFunction",
				Receiver:    "",
				Parameters:  nil,
				ReturnTypes: nil,
				FileName:    "test.go",
				LineNumber:  4,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			node, err := parser.ParseFile(fset, "test.go", tt.src, parser.ParseComments)
			if err != nil {
				t.Fatal(err)
			}

			var method GoMethod
			ast.Inspect(node, func(n ast.Node) bool {
				if fn, ok := n.(*ast.FuncDecl); ok && fn.Name.Name == tt.funcName {
					method = extractMethodInfo(fset, "test.go", fn)
					return false
				}
				return true
			})

			if method.Name == "" {
				t.Fatal("Method not found")
			}

			if method.Name != tt.expected.Name {
				t.Errorf("Expected name %q, got %q", tt.expected.Name, method.Name)
			}
			if method.Receiver != tt.expected.Receiver {
				t.Errorf("Expected receiver %q, got %q", tt.expected.Receiver, method.Receiver)
			}
			if !reflect.DeepEqual(method.Parameters, tt.expected.Parameters) {
				t.Errorf("Expected parameters %v, got %v", tt.expected.Parameters, method.Parameters)
			}
			if !reflect.DeepEqual(method.ReturnTypes, tt.expected.ReturnTypes) {
				t.Errorf("Expected return types %v, got %v", tt.expected.ReturnTypes, method.ReturnTypes)
			}
			if method.FileName != tt.expected.FileName {
				t.Errorf("Expected file name %q, got %q", tt.expected.FileName, method.FileName)
			}
			if method.LineNumber != tt.expected.LineNumber {
				t.Errorf("Expected line number %d, got %d", tt.expected.LineNumber, method.LineNumber)
			}
		})
	}
}

func TestASTAnalysisWithSampleCode(t *testing.T) {
	// Use the sample client code in testdata
	sampleFile := filepath.Join("testdata", "sample_client.go")

	// Parse the sample file
	methods, err := parseGoFile(sampleFile)
	if err != nil {
		t.Fatalf("Failed to parse sample file: %v", err)
	}

	// Expected methods (client methods only)
	expectedMethods := map[string]bool{
		"TranslateText":            true,
		"TranslateTextWithContext": true,
		"GetLanguages":             true,
		"GetLanguagesWithContext":  true,
		"GetTargetLanguages":       true,
		"GetSourceLanguages":       true,
		"Rephrase":                 true,
		"RephraseWithContext":      true,
		"RephraseWithOptions":      true,
		"GetUsage":                 true,
		"GetUsageWithContext":      true,
	}

	// Check that all expected methods are found
	foundMethods := make(map[string]bool)
	for _, method := range methods {
		foundMethods[method.Name] = true
		if expectedMethods[method.Name] {
			// Verify receiver is *Client
			if method.Receiver != "*Client" {
				t.Errorf("Method %s: expected receiver '*Client', got %s", method.Name, method.Receiver)
			}
		}
	}

	// Check all expected methods are present
	for methodName := range expectedMethods {
		if !foundMethods[methodName] {
			t.Errorf("Expected method %s not found", methodName)
		}
	}

	// Check that NonClientFunction is not included
	if foundMethods["NonClientFunction"] {
		t.Error("NonClientFunction should not be detected as client method")
	}

	// Verify total count (should be 11 client methods)
	if len(methods) != 11 {
		t.Errorf("Expected 11 client methods, got %d", len(methods))
	}
}

// TestEndpointMappingWithSampleCode tests endpoint mapping using real sample client code.
func TestEndpointMappingWithSampleCode(t *testing.T) {
	// Parse sample client code
	sampleFile := filepath.Join("testdata", "sample_client.go")
	methods, err := parseGoFile(sampleFile)
	if err != nil {
		t.Fatalf("Failed to parse sample file: %v", err)
	}

	// Create sample OpenAPI spec
	spec := &OpenAPISpec{
		Info: struct {
			Title   string `yaml:"title"`
			Version string `yaml:"version"`
		}{
			Title:   "DeepL API",
			Version: "2.0",
		},
		Paths: map[string]PathItem{
			"/v2/translate": {
				Post: &Operation{
					OperationID: "translateText",
					Summary:     "Translate text",
					Tags:        []string{"translation"},
				},
			},
			"/v2/languages": {
				Get: &Operation{
					OperationID: "getLanguages",
					Summary:     "Get supported languages",
					Tags:        []string{"languages"},
				},
			},
			"/v2/usage": {
				Get: &Operation{
					OperationID: "getUsage",
					Summary:     "Get API usage",
					Tags:        []string{"usage"},
				},
			},
			"/v2/write/rephrase": {
				Post: &Operation{
					OperationID: "rephrase",
					Summary:     "Rephrase text",
					Tags:        []string{"writing"},
				},
			},
		},
	}

	// Extract endpoints
	endpoints := extractEndpoints(spec)

	// Create mappings
	mappings := createEndpointMappings(endpoints, methods)

	// Verify mappings
	expectedMappings := map[string]string{
		"/v2/translate":      "TranslateText",
		"/v2/languages":      "GetLanguages",
		"/v2/usage":          "GetUsage",
		"/v2/write/rephrase": "Rephrase",
	}

	for _, mapping := range mappings {
		expectedMethod, exists := expectedMappings[mapping.APIEndpoint]
		if exists {
			if !mapping.IsImplemented {
				t.Errorf("Endpoint %s should be implemented", mapping.APIEndpoint)
			}
			if mapping.GoMethod == nil || mapping.GoMethod.Name != expectedMethod {
				t.Errorf("Endpoint %s: expected method %s, got %v", mapping.APIEndpoint, expectedMethod, mapping.GoMethod)
			}
		}
	}

	// Check total endpoints
	if len(mappings) != 4 {
		t.Errorf("Expected 4 endpoint mappings, got %d", len(mappings))
	}
}

func TestCreateEndpointMappings(t *testing.T) {
	// Create sample endpoints
	endpoints := []EndpointMapping{
		{
			APIEndpoint:   "/v2/translate",
			HTTPMethod:    "POST",
			OperationID:   "translateText",
			Description:   "Translate text",
			Category:      "translation",
			Priority:      "High",
			IsImplemented: false,
		},
		{
			APIEndpoint:   "/v2/languages",
			HTTPMethod:    "GET",
			OperationID:   "getLanguages",
			Description:   "Get supported languages",
			Category:      "languages",
			Priority:      "High",
			IsImplemented: false,
		},
		{
			APIEndpoint:   "/v2/unknown",
			HTTPMethod:    "GET",
			OperationID:   "unknownOp",
			Description:   "Unknown endpoint",
			Category:      "utilities",
			Priority:      "Low",
			IsImplemented: false,
		},
	}

	// Create sample methods
	methods := []GoMethod{
		{
			Name:     "TranslateText",
			Receiver: "*Client",
		},
		{
			Name:     "GetLanguages",
			Receiver: "*Client",
		},
		{
			Name:     "SomeOtherMethod",
			Receiver: "*Client",
		},
	}

	// Test createEndpointMappings
	mappings := createEndpointMappings(endpoints, methods)

	// Verify results
	if len(mappings) != 3 {
		t.Errorf("Expected 3 mappings, got %d", len(mappings))
	}

	// Check implemented endpoints
	implementedCount := 0
	for _, mapping := range mappings {
		if mapping.IsImplemented {
			implementedCount++
			if mapping.GoMethod == nil {
				t.Error("Implemented mapping should have GoMethod")
			}
		}
	}

	if implementedCount != 2 {
		t.Errorf("Expected 2 implemented endpoints, got %d", implementedCount)
	}

	// Check specific mappings
	for _, mapping := range mappings {
		if mapping.APIEndpoint == "/v2/translate" {
			if !mapping.IsImplemented || mapping.GoMethod.Name != "TranslateText" {
				t.Errorf("Translate endpoint should be mapped to TranslateText")
			}
		}
		if mapping.APIEndpoint == "/v2/languages" {
			if !mapping.IsImplemented || mapping.GoMethod.Name != "GetLanguages" {
				t.Errorf("Languages endpoint should be mapped to GetLanguages")
			}
		}
		if mapping.APIEndpoint == "/v2/unknown" {
			if mapping.IsImplemented {
				t.Errorf("Unknown endpoint should not be implemented")
			}
		}
	}
}

func TestMatchMethodToEndpoint(t *testing.T) {
	// Create sample methods
	methods := []GoMethod{
		{
			Name:     "TranslateText",
			Receiver: "*Client",
		},
		{
			Name:     "GetLanguages",
			Receiver: "*Client",
		},
		{
			Name:     "Rephrase",
			Receiver: "*Client",
		},
		{
			Name:     "GetUsage",
			Receiver: "*Client",
		},
	}

	tests := []struct {
		name     string
		endpoint EndpointMapping
		expected *string // nil if no match, method name if match
	}{
		{
			name: "Exact operation ID match",
			endpoint: EndpointMapping{
				OperationID: "translateText",
			},
			expected: stringPtr("TranslateText"),
		},
		{
			name: "Path-based match for languages",
			endpoint: EndpointMapping{
				APIEndpoint: "/v2/languages",
				OperationID: "", // No operation ID
			},
			expected: stringPtr("GetLanguages"),
		},
		{
			name: "Path-based match for rephrase",
			endpoint: EndpointMapping{
				APIEndpoint: "/v2/write/rephrase",
				OperationID: "",
			},
			expected: stringPtr("Rephrase"),
		},
		{
			name: "No match",
			endpoint: EndpointMapping{
				APIEndpoint: "/v2/unknown",
				OperationID: "unknownOp",
			},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchMethodToEndpoint(tt.endpoint, methods)
			if tt.expected == nil {
				if result != nil {
					t.Errorf("Expected no match, got %s", result.Name)
				}
			} else {
				if result == nil {
					t.Errorf("Expected match with %s, got nil", *tt.expected)
				} else if result.Name != *tt.expected {
					t.Errorf("Expected match with %s, got %s", *tt.expected, result.Name)
				}
			}
		})
	}
}

// String helpers tests
// ----------------------------------------------------------------------------

func TestOperationIDToMethodName(t *testing.T) {
	tests := []struct {
		operationID string
		expected    string
	}{
		{"translateText", "TranslateText"},
		{"getLanguages", "GetLanguages"},
		{"getUsage", "GetUsage"},
		{"rephrase", "Rephrase"},
		{"createGlossary", "CreateGlossary"},
		{"deleteGlossary", "DeleteGlossary"},
		{"", ""},
		{"unknown", "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.operationID, func(t *testing.T) {
			result := operationIDToMethodName(tt.operationID)
			if result != tt.expected {
				t.Errorf("operationIDToMethodName(%q) = %q, want %q", tt.operationID, result, tt.expected)
			}
		})
	}
}

func TestPathMatchesMethod(t *testing.T) {
	tests := []struct {
		path       string
		methodName string
		expected   bool
	}{
		{"/v2/translate", "TranslateText", true},
		{"/v2/translate", "GetLanguages", false},
		{"/v2/languages", "GetLanguages", true},
		{"/v2/languages", "TranslateText", false},
		{"/v2/usage", "GetUsage", true},
		{"/v2/usage", "TranslateText", false},
		{"/v2/write/rephrase", "Rephrase", true},
		{"/v2/write/rephrase", "TranslateText", false},
		{"/v2/unknown", "SomeMethod", false},
		{"/v2/admin/settings", "AdminSettings", false}, // No matching logic for admin
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s_%s", tt.path, tt.methodName), func(t *testing.T) {
			result := pathMatchesMethod(tt.path, tt.methodName)
			if result != tt.expected {
				t.Errorf("pathMatchesMethod(%q, %q) = %v, want %v", tt.path, tt.methodName, result, tt.expected)
			}
		})
	}
}

// TestMarkdownReportGenerator_Generate tests report generation with sample data.
func TestMarkdownReportGenerator_Generate(t *testing.T) {
	// Create sample data
	mappings := []EndpointMapping{
		{
			APIEndpoint:   "/v2/translate",
			HTTPMethod:    "POST",
			OperationID:   "translateText",
			Description:   "Translate text",
			Category:      "translation",
			Priority:      "High",
			IsImplemented: true,
			GoMethod: &GoMethod{
				Name:     "TranslateText",
				Receiver: "*Client",
				Comments: "Translates text from source to target language",
			},
		},
		{
			APIEndpoint:   "/v2/languages",
			HTTPMethod:    "GET",
			OperationID:   "getLanguages",
			Description:   "Get supported languages",
			Category:      "languages",
			Priority:      "High",
			IsImplemented: true,
			GoMethod: &GoMethod{
				Name:     "GetLanguages",
				Receiver: "*Client",
			},
		},
		{
			APIEndpoint:   "/v2/usage",
			HTTPMethod:    "GET",
			OperationID:   "getUsage",
			Description:   "Get API usage",
			Category:      "usage",
			Priority:      "Medium",
			IsImplemented: false,
		},
	}

	methods := []GoMethod{
		{
			Name:        "TranslateText",
			Receiver:    "*Client",
			Parameters:  []string{"text string", "opts *TranslateOptions"},
			ReturnTypes: []string{"*TranslateResponse", "error"},
			FileName:    "translate_text.go",
			Comments:    "Translates text from source to target language",
		},
		{
			Name:        "GetLanguages",
			Receiver:    "*Client",
			Parameters:  []string{},
			ReturnTypes: []string{"[]string", "error"},
			FileName:    "languages.go",
		},
	}

	categories := map[string][]EndpointMapping{
		"translation": {mappings[0]},
		"languages":   {mappings[1]},
		"usage":       {mappings[2]},
	}

	// Generate report
	generator := &MarkdownReportGenerator{}
	report := generator.Generate(mappings, methods, categories)

	// Basic checks
	if report == "" {
		t.Error("Report should not be empty")
	}

	// Check header
	if !strings.Contains(report, "# DeepL API Coverage Report") {
		t.Error("Report should contain main header")
	}

	// Check executive summary
	if !strings.Contains(report, "## Executive Summary") {
		t.Error("Report should contain executive summary")
	}
	if !strings.Contains(report, "Total API Endpoints**: 3") {
		t.Error("Report should show correct total endpoints")
	}
	if !strings.Contains(report, "Implemented Endpoints**: 2") {
		t.Error("Report should show correct implemented endpoints")
	}

	// Check coverage by category
	if !strings.Contains(report, "## Coverage by Category") {
		t.Error("Report should contain coverage by category")
	}
	if !strings.Contains(report, "| translation | 1 | 1 | 100.0% |") {
		t.Error("Report should show translation category coverage")
	}

	// Check implemented endpoints
	if !strings.Contains(report, "### ✅ Implemented Endpoints") {
		t.Error("Report should contain implemented endpoints section")
	}
	if !strings.Contains(report, "**POST /v2/translate** → `TranslateText`") {
		t.Error("Report should list implemented translate endpoint")
	}

	// Check missing endpoints
	if !strings.Contains(report, "### ❌ Missing Endpoints") {
		t.Error("Report should contain missing endpoints section")
	}
	if !strings.Contains(report, "**GET /v2/usage**") {
		t.Error("Report should list missing usage endpoint")
	}

	// Check Go client methods
	if !strings.Contains(report, "## Go Client Methods") {
		t.Error("Report should contain Go client methods section")
	}
	if !strings.Contains(report, "### translate_text.go") {
		t.Error("Report should list translate_text.go file")
	}
	if !strings.Contains(report, "`TranslateText(text string, opts *TranslateOptions) (*TranslateResponse, error)`") {
		t.Error("Report should show method signature")
	}

	// Check footer
	if !strings.Contains(report, "---") {
		t.Error("Report should contain footer separator")
	}
	if !strings.Contains(report, "*Report generated on") {
		t.Error("Report should contain generation timestamp")
	}
}

// Helper function for tests
func stringPtr(s string) *string {
	return &s
}

// CoverageAnalyzer tests
// ----------------------------------------------------------------------------

// MockAPISpecFetcher for testing
type MockAPISpecFetcher struct {
	FetchFunc func() (*OpenAPISpec, error)
}

func (m *MockAPISpecFetcher) Fetch() (*OpenAPISpec, error) {
	if m.FetchFunc != nil {
		return m.FetchFunc()
	}
	return &OpenAPISpec{}, nil
}

// MockGoSourceAnalyzer for testing
type MockGoSourceAnalyzer struct {
	AnalyzeFunc func(rootDir string) ([]GoMethod, error)
}

func (m *MockGoSourceAnalyzer) Analyze(rootDir string) ([]GoMethod, error) {
	if m.AnalyzeFunc != nil {
		return m.AnalyzeFunc(rootDir)
	}
	return []GoMethod{}, nil
}

// MockMarkdownReportGenerator for testing
type MockMarkdownReportGenerator struct {
	GenerateFunc func(mappings []EndpointMapping, methods []GoMethod, categories map[string][]EndpointMapping) string
	SaveFunc     func(filename, content string) error
}

func (m *MockMarkdownReportGenerator) Generate(mappings []EndpointMapping, methods []GoMethod, categories map[string][]EndpointMapping) string {
	if m.GenerateFunc != nil {
		return m.GenerateFunc(mappings, methods, categories)
	}
	return "mock report"
}

func (m *MockMarkdownReportGenerator) Save(filename, content string) error {
	if m.SaveFunc != nil {
		return m.SaveFunc(filename, content)
	}
	return nil
}

// TestCoverageAnalyzer_Run tests the main Run workflow for fail-fast detection of DeepL spec changes.
func TestCoverageAnalyzer_Run(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T) (*CoverageAnalyzer, string, string)
		expectError bool
		errorMsg    string
		validate    func(t *testing.T, reportPath string)
	}{
		{
			name: "successful run",
			setup: func(t *testing.T) (*CoverageAnalyzer, string, string) {
				tempDir := t.TempDir()
				reportPath := filepath.Join(tempDir, "report.md")

				mockSpecFetcher := &MockAPISpecFetcher{
					FetchFunc: func() (*OpenAPISpec, error) {
						return &OpenAPISpec{
							Info: struct {
								Title   string `yaml:"title"`
								Version string `yaml:"version"`
							}{
								Title:   "DeepL API",
								Version: "1.0.0",
							},
							Paths: map[string]PathItem{
								"/v2/translate": {
									Post: &Operation{
										OperationID: "translateText",
										Summary:     "Translate text",
									},
								},
							},
						}, nil
					},
				}

				mockSourceAnalyzer := &MockGoSourceAnalyzer{
					AnalyzeFunc: func(rootDir string) ([]GoMethod, error) {
						return []GoMethod{
							{
								Name:     "TranslateText",
								Receiver: "*Client",
							},
						}, nil
					},
				}

				mockReportGenerator := &MockMarkdownReportGenerator{
					GenerateFunc: func(mappings []EndpointMapping, methods []GoMethod, categories map[string][]EndpointMapping) string {
						return "# Test Report\n\nCoverage: 100%"
					},
					SaveFunc: func(filename, content string) error {
						return os.WriteFile(filename, []byte(content), 0644)
					},
				}

				analyzer := &CoverageAnalyzer{
					SpecFetcher:     mockSpecFetcher,
					SourceAnalyzer:  mockSourceAnalyzer,
					ReportGenerator: mockReportGenerator,
					Logger:          func(format string, args ...interface{}) {},
				}

				return analyzer, tempDir, reportPath
			},
			expectError: false,
			validate: func(t *testing.T, reportPath string) {
				if _, err := os.Stat(reportPath); os.IsNotExist(err) {
					t.Error("Expected report file to be created")
				}
				content, err := os.ReadFile(reportPath)
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(content), "# Test Report") {
					t.Error("Expected report content to be saved")
				}
			},
		},
		{
			name: "spec fetch error",
			setup: func(t *testing.T) (*CoverageAnalyzer, string, string) {
				tempDir := t.TempDir()
				reportPath := filepath.Join(tempDir, "report.md")

				mockSpecFetcher := &MockAPISpecFetcher{
					FetchFunc: func() (*OpenAPISpec, error) {
						return nil, fmt.Errorf("failed to fetch spec")
					},
				}

				analyzer := &CoverageAnalyzer{
					SpecFetcher: mockSpecFetcher,
					Logger:      func(format string, args ...interface{}) {},
				}

				return analyzer, tempDir, reportPath
			},
			expectError: true,
			errorMsg:    "failed to fetch OpenAPI spec",
		},
		{
			name: "source analysis error",
			setup: func(t *testing.T) (*CoverageAnalyzer, string, string) {
				tempDir := t.TempDir()
				reportPath := filepath.Join(tempDir, "report.md")

				mockSpecFetcher := &MockAPISpecFetcher{
					FetchFunc: func() (*OpenAPISpec, error) {
						return &OpenAPISpec{}, nil
					},
				}

				mockSourceAnalyzer := &MockGoSourceAnalyzer{
					AnalyzeFunc: func(rootDir string) ([]GoMethod, error) {
						return nil, fmt.Errorf("analysis failed")
					},
				}

				analyzer := &CoverageAnalyzer{
					SpecFetcher:    mockSpecFetcher,
					SourceAnalyzer: mockSourceAnalyzer,
					Logger:         func(format string, args ...interface{}) {},
				}

				return analyzer, tempDir, reportPath
			},
			expectError: true,
			errorMsg:    "failed to analyze Go source code",
		},
		{
			name: "report save error",
			setup: func(t *testing.T) (*CoverageAnalyzer, string, string) {
				tempDir := t.TempDir()
				reportPath := filepath.Join(tempDir, "report.md")

				mockSpecFetcher := &MockAPISpecFetcher{
					FetchFunc: func() (*OpenAPISpec, error) {
						return &OpenAPISpec{}, nil
					},
				}

				mockSourceAnalyzer := &MockGoSourceAnalyzer{
					AnalyzeFunc: func(rootDir string) ([]GoMethod, error) {
						return []GoMethod{}, nil
					},
				}

				mockReportGenerator := &MockMarkdownReportGenerator{
					GenerateFunc: func(mappings []EndpointMapping, methods []GoMethod, categories map[string][]EndpointMapping) string {
						return "test report"
					},
					SaveFunc: func(filename, content string) error {
						return fmt.Errorf("save failed")
					},
				}

				analyzer := &CoverageAnalyzer{
					SpecFetcher:     mockSpecFetcher,
					SourceAnalyzer:  mockSourceAnalyzer,
					ReportGenerator: mockReportGenerator,
					Logger:          func(format string, args ...interface{}) {},
				}

				return analyzer, tempDir, reportPath
			},
			expectError: true,
			errorMsg:    "failed to save report",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analyzer, sourceRoot, reportPath := tt.setup(t)
			err := analyzer.Run(sourceRoot, reportPath)
			if tt.expectError {
				if err == nil || !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error containing %q, got %v", tt.errorMsg, err)
				}
			} else {
				if err != nil {
					t.Fatalf("Expected no error, got %v", err)
				}
				if tt.validate != nil {
					tt.validate(t, reportPath)
				}
			}
		})
	}
}

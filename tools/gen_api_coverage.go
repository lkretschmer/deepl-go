//go:generate go run .

// Package main generates API coverage analysis for the deepl-go library.
//
// This tool downloads the official DeepL API OpenAPI specification from GitHub,
// analyzes the current Go client implementation using AST parsing, and generates
// a comprehensive coverage report in Markdown format.
//
// The analysis includes:
//   - Endpoint mapping between API specs and Go methods
//   - Coverage statistics by category
//   - Implementation priority recommendations
//   - Detailed method signatures and documentation
//
// Generated files:
//   - api_coverage_report.md: Coverage analysis report
//   - tools/testdata/openapi_spec.yaml: Cached OpenAPI specification
//
// Usage:
//
//	go run ./tools/gen_api_coverage.go
//
// Or from project root:
//
//	go generate ./tools
package main

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Constants/Variables
// ----------------------------------------------------------------------------

// Configuration constants for the generator. We do not use config file for simplicity.
const (
	coverageReportFile = "api_coverage_report.md"
	httpTimeoutSeconds = 30
	openAPISpecFile    = "openapi_spec.yaml"
	openAPISpecURL     = "https://raw.githubusercontent.com/DeepLcom/openapi/main/openapi.yaml"
	sourceCodeRoot     = "." // Current directory (project root)
	toolsDir           = "tools"
	testDataDir        = "testdata"
	userAgent          = "deepl-go-coverage-analyzer/1.0"
)

var (
	// path to store the generated coverage report (to project root.)
	coverageReportFilePath = filepath.Join(sourceCodeRoot, coverageReportFile)
	// path to store/cache the OpenAPI specification locally (relative to project root.)
	openAPISpecFilePath = filepath.Join(sourceCodeRoot, toolsDir, testDataDir, openAPISpecFile)
)

// Interface definitions
// ----------------------------------------------------------------------------

// APISpecFetcherInterface defines the interface for fetching OpenAPI specifications.
type APISpecFetcherInterface interface {
	Fetch() (*OpenAPISpec, error)
}

// GoSourceAnalyzerInterface defines the interface for analyzing Go source code.
type GoSourceAnalyzerInterface interface {
	Analyze(rootDir string) ([]GoMethod, error)
}

// ReportGeneratorInterface defines the interface for generating and saving reports.
type ReportGeneratorInterface interface {
	Generate(mappings []EndpointMapping, methods []GoMethod, categories map[string][]EndpointMapping) string
	Save(filename, content string) error
}

// FileWalker defines the interface for walking through files.
type FileWalker interface {
	Walk(root string, walkFn filepath.WalkFunc) error
}

// Struct definitions
// ----------------------------------------------------------------------------

// OpenAPISpec represents the basic structure of the DeepL OpenAPI specification.
type OpenAPISpec struct {
	Info struct {
		Title   string `yaml:"title"`
		Version string `yaml:"version"`
	} `yaml:"info"`
	Paths map[string]PathItem `yaml:"paths"`
}

// PathItem holds the operations for a specific API endpoint.
type PathItem struct {
	Get    *Operation `yaml:"get,omitempty"`
	Post   *Operation `yaml:"post,omitempty"`
	Put    *Operation `yaml:"put,omitempty"`
	Delete *Operation `yaml:"delete,omitempty"`
}

// Operation represents an API operation (HTTP method + endpoint.)
type Operation struct {
	OperationID string              `yaml:"operationId"`
	Summary     string              `yaml:"summary"`
	Description string              `yaml:"description"`
	Parameters  []Parameter         `yaml:"parameters,omitempty"`
	RequestBody *RequestBody        `yaml:"requestBody,omitempty"`
	Responses   map[string]Response `yaml:"responses"`
	Tags        []string            `yaml:"tags,omitempty"`
}

// Parameter holds the information about an API parameter used in operations.
type Parameter struct {
	Name        string `yaml:"name"`
	In          string `yaml:"in"` // query, header, path, etc.
	Required    bool   `yaml:"required"`
	Description string `yaml:"description"`
	Schema      Schema `yaml:"schema"`
}

// RequestBody represents request body specification.
type RequestBody struct {
	Required    bool                 `yaml:"required"`
	Description string               `yaml:"description"`
	Content     map[string]MediaType `yaml:"content"`
}

// Response represents response specification.
type Response struct {
	Description string               `yaml:"description"`
	Content     map[string]MediaType `yaml:"content,omitempty"`
}

// MediaType represents content type specification.
type MediaType struct {
	Schema Schema `yaml:"schema"`
}

// Schema represents simplified data schema. Minimal structure for our analysis.
type Schema struct {
	Type        string            `yaml:"type"`
	Format      string            `yaml:"format,omitempty"`
	Description string            `yaml:"description,omitempty"`
	Properties  map[string]Schema `yaml:"properties,omitempty"`
}

// GoMethod holds information about a detected Go method in the client code.
type GoMethod struct {
	Name        string   // Method name (e.g., "TranslateText")
	Receiver    string   // Receiver type (e.g., "*Client")
	Parameters  []string // Parameter names and types
	ReturnTypes []string // Return value types
	FileName    string   // Source file containing the method
	LineNumber  int      // Line number where method is defined
	Comments    string   // Associated documentation comments
}

// EndpointMapping represents the relationship between API endpoints and Go methods.
type EndpointMapping struct {
	APIEndpoint   string    // API path (e.g., "/v2/translate")
	HTTPMethod    string    // HTTP method (GET, POST, etc.)
	OperationID   string    // OpenAPI operation ID
	GoMethod      *GoMethod // Corresponding Go method (nil if not implemented)
	Priority      string    // Implementation priority (High/Medium/Low)
	Category      string    // Functional category (Translation, Languages, etc.)
	Description   string    // Human-readable description
	IsImplemented bool      // Whether this endpoint is implemented
}

// CoverageReport holds the final analysis results to be included in the report.
type CoverageReport struct {
	GeneratedAt        time.Time         // Report generation timestamp
	OpenAPIVersion     string            // Version of the OpenAPI spec used
	TotalEndpoints     int               // Total number of API endpoints
	ImplementedCount   int               // Number of implemented endpoints
	CoveragePercent    float64           // Implementation coverage percentage
	Mappings           []EndpointMapping // Detailed endpoint mappings
	ImplementedMethods []GoMethod        // All detected Go methods
	MissingEndpoints   []EndpointMapping // Prioritized list of unimplemented endpoints
}

// APISpecFetcher handles fetching OpenAPI specifications.
// It is an implementation of APISpecFetcherInterface.
type APISpecFetcher struct {
	HTTPClient *http.Client
	URL        string
	CachePath  string
	Timeout    time.Duration
	Logger     func(format string, args ...any)
}

// GoSourceAnalyzer handles analysis of Go source code.
// It is an implementation of GoSourceAnalyzerInterface.
type GoSourceAnalyzer struct {
	FileWalker FileWalker
	Logger     func(format string, args ...any)
}

// OSFileWalker implements FileWalker using os package.
// It is an implementation of FileWalker interface.
type OSFileWalker struct{}

// MarkdownReportGenerator handles generating and saving Markdown reports.
type MarkdownReportGenerator struct{}

// CoverageAnalyzer orchestrates the entire API coverage analysis process.
type CoverageAnalyzer struct {
	SpecFetcher     APISpecFetcherInterface
	SourceAnalyzer  GoSourceAnalyzerInterface
	ReportGenerator ReportGeneratorInterface
	Logger          func(format string, args ...any)
}

// Constructor functions
// ----------------------------------------------------------------------------

// NewAPISpecFetcher creates a new APISpecFetcher with default settings.
func NewAPISpecFetcher() *APISpecFetcher {
	return &APISpecFetcher{
		HTTPClient: &http.Client{},
		URL:        openAPISpecURL,
		CachePath:  openAPISpecFilePath,
		Timeout:    time.Duration(httpTimeoutSeconds) * time.Second,
		Logger: func(format string, args ...interface{}) {
			fmt.Printf(format, args...)
		},
	}
}

// NewCoverageAnalyzer creates a new CoverageAnalyzer with default settings.
func NewCoverageAnalyzer() *CoverageAnalyzer {
	return &CoverageAnalyzer{
		SpecFetcher:     NewAPISpecFetcher(),
		SourceAnalyzer:  NewGoSourceAnalyzer(),
		ReportGenerator: &MarkdownReportGenerator{},
		Logger: func(format string, args ...interface{}) {
			fmt.Printf(format, args...)
		},
	}
}

// NewGoSourceAnalyzer creates a new GoSourceAnalyzer with default settings.
func NewGoSourceAnalyzer() *GoSourceAnalyzer {
	return &GoSourceAnalyzer{
		FileWalker: &OSFileWalker{},
		Logger: func(format string, args ...interface{}) {
			fmt.Printf(format, args...)
		},
	}
}

// Public methods
// ----------------------------------------------------------------------------

// Fetch downloads the latest OpenAPI specification from DeepL's repository.
func (f *APISpecFetcher) Fetch() (*OpenAPISpec, error) {
	f.Logger("🌐 Fetching OpenAPI specification from DeepL...")

	// Check if cached file exists and is recent (less than 1 hour old)
	if cachedSpec, err := f.loadCachedSpec(); err == nil {
		f.Logger("📁 Using cached OpenAPI specification")
		return cachedSpec, nil
	}

	// Create HTTP client with timeout
	ctx, cancel := context.WithTimeout(context.Background(), f.Timeout)

	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", f.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set user agent
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/yaml, text/yaml, */*")

	// Make HTTP request
	resp, err := f.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch OpenAPI spec: %w", err)
	}

	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			f.Logger("Warning: failed to close response body: %v", closeErr)
		}
	}()

	// Check HTTP status
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP error: %d %s", resp.StatusCode, resp.Status)
	}

	// Limit response size to prevent DoS (5MB max)
	const maxResponseSize = 5 * 1024 * 1024
	limitedReader := io.LimitReader(resp.Body, maxResponseSize)

	// Read response body
	yamlContent, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	f.Logger("📦 Downloaded %d bytes of OpenAPI specification", len(yamlContent))

	// Parse YAML content
	spec, err := parseOpenAPISpec(yamlContent)
	if err != nil {
		return nil, fmt.Errorf("failed to parse OpenAPI spec: %w", err)
	}

	// Cache the spec for future use
	if err := f.cacheSpec(yamlContent); err != nil {
		// Don't fail the entire operation just because caching failed but log it
		f.Logger("⚠️  Warning: failed to cache OpenAPI spec: %v", err)
	}

	f.Logger("✅ Successfully parsed OpenAPI spec: %s v%s", spec.Info.Title, spec.Info.Version)
	return spec, nil
}

// Run executes the complete API coverage analysis process.
func (c *CoverageAnalyzer) Run(sourceCodeRoot, reportFilePath string) error {
	c.Logger("🚀 Starting DeepL API Coverage Analysis...\n")

	// Fetch OpenAPI specification
	spec, err := c.SpecFetcher.Fetch()
	if err != nil {
		return fmt.Errorf("failed to fetch OpenAPI spec: %w", err)
	}

	// Extract endpoints from OpenAPI spec
	endpoints := extractEndpoints(spec)

	// Analyze Go source code
	methods, err := c.SourceAnalyzer.Analyze(sourceCodeRoot)
	if err != nil {
		return fmt.Errorf("failed to analyze Go source code: %w", err)
	}

	// Create endpoint mappings
	mappings := createEndpointMappings(endpoints, methods)

	// Assign priorities
	assignPriorities(mappings)

	// Categorize endpoints
	categories := categorizeEndpoints(mappings)

	// Generate report
	report := c.ReportGenerator.Generate(mappings, methods, categories)

	// Save report
	if err := c.ReportGenerator.Save(reportFilePath, report); err != nil {
		return fmt.Errorf("failed to save report: %w", err)
	}

	c.Logger("✅ Coverage report generated: %s\n", reportFilePath)

	return nil
}

// Analyze scans the Go codebase to find implemented client methods.
func (a *GoSourceAnalyzer) Analyze(rootDir string) ([]GoMethod, error) {
	a.Logger("📁 Scanning Go source code in %s...\n", rootDir)

	var allMethods []GoMethod

	// Walk through all Go files in the directory
	err := a.FileWalker.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Only process .go files
		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		// Skip test files and this generator tool
		if strings.HasSuffix(path, "_test.go") ||
			strings.Contains(path, "tools/") ||
			strings.Contains(path, "tools\\") {
			return nil
		}

		a.Logger("   📄 Parsing file: %s\n", path)

		// Parse this Go file
		methods, err := parseGoFile(path)
		if err != nil {
			a.Logger("⚠️  Warning: failed to parse %s: %v\n", path, err)
			return nil // Continue processing other files
		}

		if len(methods) > 0 {
			a.Logger("      🔍 Found %d methods in %s\n", len(methods), filepath.Base(path))
			for _, method := range methods {
				a.Logger("         • %s.%s\n", method.Receiver, method.Name)
			}
		}

		allMethods = append(allMethods, methods...)

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk directory: %w", err)
	}

	a.Logger("🔍 Found %d client methods across all Go files\n", len(allMethods))

	return allMethods, nil
}

// Generate creates a detailed Markdown report of API coverage.
// Reference this method for report structure (the order of sections.)
func (g *MarkdownReportGenerator) Generate(mappings []EndpointMapping, methods []GoMethod, categories map[string][]EndpointMapping) string {
	var report strings.Builder

	report.WriteString(g.generateHeader())
	report.WriteString(g.generateExecutiveSummary(mappings, methods))
	report.WriteString(g.generateCoverageByCategory(categories))
	report.WriteString(g.generateDetailedAnalysis(mappings))
	report.WriteString(g.generateClientMethods(methods))
	report.WriteString(g.generateRecommendations())
	report.WriteString(g.generateFooter())

	return report.String()
}

// Save saves the report content to a file.
func (g *MarkdownReportGenerator) Save(filename, content string) error {
	return os.WriteFile(filename, []byte(content), 0644)
}

// Walk walks through files starting from root directory.
func (w *OSFileWalker) Walk(root string, walkFn filepath.WalkFunc) error {
	return filepath.Walk(root, walkFn)
}

// Private methods
// ----------------------------------------------------------------------------

// loadCachedSpec attempts to load and parse cached OpenAPI specification.
func (f *APISpecFetcher) loadCachedSpec() (*OpenAPISpec, error) {
	// Check if cache file exists
	info, err := os.Stat(f.CachePath)
	if err != nil {
		return nil, err // File doesn't exist or can't be accessed
	}

	// Check if cache is recent (less than 1 hour old)
	if time.Since(info.ModTime()) > time.Hour {
		return nil, fmt.Errorf("cached spec is too old")
	}

	// Read cached file
	yamlContent, err := os.ReadFile(f.CachePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read cached spec: %w", err)
	}

	// Parse cached content
	return parseOpenAPISpec(yamlContent)
}

// cacheSpec saves OpenAPI specification to local file for future use.
func (f *APISpecFetcher) cacheSpec(yamlContent []byte) error {
	// Write to file with appropriate permissions
	err := os.WriteFile(f.CachePath, yamlContent, 0644)
	if err != nil {
		return fmt.Errorf("failed to write cache file: %w", err)
	}

	f.Logger("💾 Cached OpenAPI spec to %s", f.CachePath)

	return nil
}

// generateHeader creates the header section of the report.
func (g *MarkdownReportGenerator) generateHeader() string {
	var header strings.Builder

	header.WriteString("<!-- markdownlint-disable MD041 -->\n")
	header.WriteString("> **⚠️ Code generated by go generate; DO NOT EDIT.**\n")
	header.WriteString("> Generator: [tools/gen_api_coverage.go](tools/gen_api_coverage.go)\n\n")
	header.WriteString("# DeepL API Coverage Report\n\n")
	header.WriteString("This report provides a comprehensive analysis of the DeepL API implementation coverage.\n\n")

	return header.String()
}

// generateExecutiveSummary creates the executive summary section.
func (g *MarkdownReportGenerator) generateExecutiveSummary(mappings []EndpointMapping, methods []GoMethod) string {
	var summary strings.Builder

	summary.WriteString("## Executive Summary\n\n")
	_, implemented, coverage := calculateCoverageStats(mappings)
	summary.WriteString(fmt.Sprintf("- **Total API Endpoints**: %d\n", len(mappings)))
	summary.WriteString(fmt.Sprintf("- **Implemented Endpoints**: %d\n", implemented))
	summary.WriteString(fmt.Sprintf("- **Coverage Percentage**: %.1f%%\n", coverage))
	summary.WriteString(fmt.Sprintf("- **Go Client Methods**: %d\n\n", len(methods)))

	return summary.String()
}

// generateCoverageByCategory creates the coverage by category section.
func (g *MarkdownReportGenerator) generateCoverageByCategory(categories map[string][]EndpointMapping) string {
	var coverage strings.Builder

	coverage.WriteString("## Coverage by Category\n\n")
	coverage.WriteString("| Category | Implemented | Total | Coverage |\n")
	coverage.WriteString("|----------|-------------|-------|----------|\n")

	for category, categoryMappings := range categories {
		categoryImplemented, categoryTotal, categoryCoverage := calculateCategoryCoverage(categoryMappings)
		coverage.WriteString(fmt.Sprintf("| %s | %d | %d | %.1f%% |\n",
			category, categoryImplemented, categoryTotal, categoryCoverage))
	}
	coverage.WriteString("\n")

	return coverage.String()
}

// generateDetailedAnalysis creates the detailed analysis section.
func (g *MarkdownReportGenerator) generateDetailedAnalysis(mappings []EndpointMapping) string {
	var analysis strings.Builder

	analysis.WriteString("## Detailed Analysis\n\n")

	// Implemented Endpoints
	analysis.WriteString("### ✅ Implemented Endpoints\n\n")
	for _, m := range mappings {
		if m.IsImplemented {
			analysis.WriteString(fmt.Sprintf("- **%s %s** → `%s`\n",
				m.HTTPMethod, m.APIEndpoint, m.GoMethod.Name))
			if m.GoMethod.Comments != "" {
				analysis.WriteString(fmt.Sprintf("  - %s\n", m.GoMethod.Comments))
			}
		}
	}

	analysis.WriteString("\n")

	// Missing Endpoints
	analysis.WriteString("### ❌ Missing Endpoints\n\n")

	// Group by priority
	priorities := []string{"High", "Medium", "Low"}
	for _, priority := range priorities {
		hasItems := false
		for _, m := range mappings {
			if !m.IsImplemented && m.Priority == priority {
				if !hasItems {
					analysis.WriteString(fmt.Sprintf("#### %s Priority\n\n", priority))
					hasItems = true
				}

				analysis.WriteString(fmt.Sprintf("- **%s %s**\n", m.HTTPMethod, m.APIEndpoint))

				if m.Description != "" {
					analysis.WriteString(fmt.Sprintf("  - %s\n", m.Description))
				}
			}
		}
		if hasItems {
			analysis.WriteString("\n")
		}
	}

	return analysis.String()
}

// generateClientMethods creates the Go client methods section.
func (g *MarkdownReportGenerator) generateClientMethods(methods []GoMethod) string {
	var clientMethods strings.Builder

	clientMethods.WriteString("## Go Client Methods\n\n")
	clientMethods.WriteString("The following methods were detected in the Go client:\n\n")

	methodsByFile := make(map[string][]GoMethod)
	for _, method := range methods {
		methodsByFile[method.FileName] = append(methodsByFile[method.FileName], method)
	}

	for filename, fileMethods := range methodsByFile {
		clientMethods.WriteString(fmt.Sprintf("### %s\n\n", filename))
		for _, method := range fileMethods {
			methodEntry := fmt.Sprintf("- `%s(%s) (%s)`\n",
				method.Name,
				strings.Join(method.Parameters, ", "),
				strings.Join(method.ReturnTypes, ", "),
			)

			clientMethods.WriteString(methodEntry)

			if method.Comments != "" {
				clientMethods.WriteString(fmt.Sprintf("  - %s\n", method.Comments))
			}
		}
		clientMethods.WriteString("\n")
	}

	return clientMethods.String()
}

// generateRecommendations creates the recommendations section.
func (g *MarkdownReportGenerator) generateRecommendations() string {
	var recommendations strings.Builder

	recommendations.WriteString("## Recommendations\n\n")
	recommendations.WriteString("Based on this analysis, the following implementation priorities are suggested:\n\n")
	recommendations.WriteString("1. **High Priority**: Focus on core translation and language detection features\n")
	recommendations.WriteString("2. **Medium Priority**: Implement document translation and glossary management\n")
	recommendations.WriteString("3. **Low Priority**: Add administrative and advanced configuration features\n\n")

	return recommendations.String()
}

// generateFooter creates the footer section.
func (g *MarkdownReportGenerator) generateFooter() string {
	var footer strings.Builder

	footer.WriteString("---\n")
	footer.WriteString(fmt.Sprintf("*Report generated on %s*\n", time.Now().Format("2006-01-02 15:04:05")))

	return footer.String()
}

// Utility functions
// ----------------------------------------------------------------------------

// OpenAPI parsing
// ---------------------------

// parseOpenAPISpec parses YAML content into structured OpenAPI specification.
func parseOpenAPISpec(yamlContent []byte) (*OpenAPISpec, error) {
	if len(yamlContent) == 0 {
		return nil, fmt.Errorf("empty YAML content")
	}

	var spec OpenAPISpec
	if err := yaml.Unmarshal(yamlContent, &spec); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Validate required fields
	if spec.Info.Title == "" {
		return nil, fmt.Errorf("missing required field: info.title")
	}

	if spec.Paths == nil {
		spec.Paths = make(map[string]PathItem)
	}

	return &spec, nil
}

// extractEndpoints converts OpenAPI paths into normalized endpoint list.
func extractEndpoints(spec *OpenAPISpec) []EndpointMapping {
	if spec == nil || spec.Paths == nil {
		return []EndpointMapping{}
	}

	var endpoints []EndpointMapping

	for path, pathItem := range spec.Paths {
		// Extract GET operation
		if pathItem.Get != nil {
			endpoints = append(endpoints, EndpointMapping{
				APIEndpoint:   path,
				HTTPMethod:    "GET",
				OperationID:   pathItem.Get.OperationID,
				Description:   pathItem.Get.Summary,
				Category:      categorizeFromPath(path),
				Priority:      "Medium", // Default priority
				IsImplemented: false,
			})
		}

		// Extract POST operation
		if pathItem.Post != nil {
			endpoints = append(endpoints, EndpointMapping{
				APIEndpoint:   path,
				HTTPMethod:    "POST",
				OperationID:   pathItem.Post.OperationID,
				Description:   pathItem.Post.Summary,
				Category:      categorizeFromPath(path),
				Priority:      "Medium", // Default priority
				IsImplemented: false,
			})
		}

		// Extract PUT operation
		if pathItem.Put != nil {
			endpoints = append(endpoints, EndpointMapping{
				APIEndpoint:   path,
				HTTPMethod:    "PUT",
				OperationID:   pathItem.Put.OperationID,
				Description:   pathItem.Put.Summary,
				Category:      categorizeFromPath(path),
				Priority:      "Medium", // Default priority
				IsImplemented: false,
			})
		}

		// Extract DELETE operation
		if pathItem.Delete != nil {
			endpoints = append(endpoints, EndpointMapping{
				APIEndpoint:   path,
				HTTPMethod:    "DELETE",
				OperationID:   pathItem.Delete.OperationID,
				Description:   pathItem.Delete.Summary,
				Category:      categorizeFromPath(path),
				Priority:      "Medium", // Default priority
				IsImplemented: false,
			})
		}
	}

	return endpoints
}

// categorizeFromPath determines category based on API path.
func categorizeFromPath(path string) string {
	switch {
	case contains(path, "translate"):
		return "translation"
	case contains(path, "language"):
		return "languages"
	case contains(path, "usage"):
		return "usage"
	case contains(path, "admin"):
		return "administration"
	default:
		return "utilities"
	}
}

// AST analysis
// ---------------------------

// parseGoFile extracts method information from a single Go file.
func parseGoFile(filename string) ([]GoMethod, error) {
	// Create file set for position tracking
	fset := token.NewFileSet()

	// Parse the Go source file
	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse file: %w", err)
	}

	var methods []GoMethod

	// Walk the AST to find function declarations
	ast.Inspect(node, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncDecl:
			// Check if this is a client method
			if isClientMethodAST(x) {
				method := extractMethodInfo(fset, filename, x)
				methods = append(methods, method)
			}
		}
		return true
	})

	return methods, nil
}

// extractMethodInfo extracts detailed information from an AST function declaration.
func extractMethodInfo(fset *token.FileSet, filename string, funcDecl *ast.FuncDecl) GoMethod {
	// Get position information
	pos := fset.Position(funcDecl.Pos())

	// Extract receiver type
	var receiver string
	if funcDecl.Recv != nil && len(funcDecl.Recv.List) > 0 {
		recv := funcDecl.Recv.List[0]
		switch t := recv.Type.(type) {
		case *ast.StarExpr:
			if ident, ok := t.X.(*ast.Ident); ok {
				receiver = "*" + ident.Name
			}
		case *ast.Ident:
			receiver = t.Name
		}
	}

	// Extract parameters
	var parameters []string
	if funcDecl.Type.Params != nil {
		for _, param := range funcDecl.Type.Params.List {
			paramType := typeToString(param.Type)
			if len(param.Names) == 0 {
				// Anonymous parameter
				parameters = append(parameters, paramType)
			} else {
				// Named parameters
				for _, name := range param.Names {
					parameters = append(parameters, name.Name+" "+paramType)
				}
			}
		}
	}

	// Extract return types
	var returnTypes []string
	if funcDecl.Type.Results != nil {
		for _, result := range funcDecl.Type.Results.List {
			returnType := typeToString(result.Type)
			returnTypes = append(returnTypes, returnType)
		}
	}

	// Extract documentation comments
	var comments string
	if funcDecl.Doc != nil {
		for _, comment := range funcDecl.Doc.List {
			comments += strings.TrimPrefix(comment.Text, "//") + " "
		}
		comments = strings.TrimSpace(comments)
	}

	return GoMethod{
		Name:        funcDecl.Name.Name,
		Receiver:    receiver,
		Parameters:  parameters,
		ReturnTypes: returnTypes,
		FileName:    filepath.Base(filename),
		LineNumber:  pos.Line,
		Comments:    comments,
	}
}

// isClientMethodAST determines if a method belongs to the DeepL client.
func isClientMethodAST(funcDecl *ast.FuncDecl) bool {
	// Check if function has a receiver
	if funcDecl.Recv == nil || len(funcDecl.Recv.List) == 0 {
		return false
	}

	// Check if method name is exported (starts with uppercase)
	if !funcDecl.Name.IsExported() {
		return false
	}

	// Get receiver type
	recv := funcDecl.Recv.List[0]

	var receiverType string

	switch t := recv.Type.(type) {
	case *ast.StarExpr:
		// Pointer receiver like *Client
		if ident, ok := t.X.(*ast.Ident); ok {
			receiverType = "*" + ident.Name
		}
	case *ast.Ident:
		// Value receiver like Client
		receiverType = t.Name
	}

	// Check if receiver type matches client patterns
	return receiverType == "*Client" || receiverType == "Client"
}

// typeToString converts an AST type expression to string representation.
func typeToString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + typeToString(t.X)
	case *ast.SelectorExpr:
		return typeToString(t.X) + "." + t.Sel.Name
	case *ast.ArrayType:
		return "[]" + typeToString(t.Elt)
	case *ast.MapType:
		return "map[" + typeToString(t.Key) + "]" + typeToString(t.Value)
	case *ast.InterfaceType:
		if len(t.Methods.List) == 0 {
			return "interface{}"
		}
		return "interface{...}"
	case *ast.ChanType:
		return "chan " + typeToString(t.Value)
	case *ast.FuncType:
		return "func(...)"
	default:
		return "unknown"
	}
}

// String helpers
// ---------------------------

// contains checks if string contains substring (case-insensitive helper.)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		(len(s) > len(substr) && findSubstring(s, substr)))
}

// findSubstring simple substring search helper.
func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			if s[i+j] != substr[j] {
				match = false

				break
			}
		}
		if match {
			return true
		}
	}

	return false
}

// operationIDToMethodName converts OpenAPI operation ID to Go method name.
func operationIDToMethodName(operationID string) string {
	if operationID == "" {
		return ""
	}

	// Convert camelCase/snake_case to PascalCase
	if operationID == "translateText" {
		return "TranslateText"
	}
	if operationID == "getLanguages" {
		return "GetLanguages"
	}
	if operationID == "getUsage" {
		return "GetUsage"
	}

	// Default: capitalize first letter
	if len(operationID) > 0 {
		return string(operationID[0]-32) + operationID[1:] // Convert first char to uppercase
	}

	return operationID
}

// pathMatchesMethod checks if API path matches Go method name.
func pathMatchesMethod(path, methodName string) bool {
	// Simple matching logic
	if contains(path, "translate") && contains(methodName, "Translate") {
		return true
	}
	if contains(path, "language") && contains(methodName, "Language") {
		return true
	}
	if contains(path, "usage") && contains(methodName, "Usage") {
		return true
	}
	if contains(path, "rephrase") && contains(methodName, "Rephrase") {
		return true
	}

	return false
}

// Coverage calculation
// ---------------------------

// calculateCategoryCoverage computes coverage for a category of endpoints.
func calculateCategoryCoverage(mappings []EndpointMapping) (int, int, float64) {
	if len(mappings) == 0 {
		return 0, 0, 0.0
	}

	implemented := 0
	for _, m := range mappings {
		if m.IsImplemented {
			implemented++
		}
	}

	coverage := float64(implemented) / float64(len(mappings)) * 100

	return implemented, len(mappings), coverage
}

// calculateCoverageStats computes coverage metrics from endpoint mappings.
func calculateCoverageStats(mappings []EndpointMapping) (int, int, float64) {
	if len(mappings) == 0 {
		return 0, 0, 0.0
	}

	total := len(mappings)
	implemented := 0

	for _, mapping := range mappings {
		if mapping.IsImplemented {
			implemented++
		}
	}

	percentage := float64(implemented) / float64(total) * 100.0

	return total, implemented, percentage
}

// Mapping and categorization
// ---------------------------

// assignPriorities determines implementation priority for missing endpoints.
func assignPriorities(mappings []EndpointMapping) {
	for i := range mappings {
		mappings[i].Priority = determinePriority(mappings[i])
	}
}

// categorizeEndpoints groups endpoints by functional area.
func categorizeEndpoints(mappings []EndpointMapping) map[string][]EndpointMapping {
	categories := make(map[string][]EndpointMapping)

	for _, mapping := range mappings {
		category := mapping.Category
		if category == "" {
			category = "utilities" // Default category
		}
		categories[category] = append(categories[category], mapping)
	}

	return categories
}

// createEndpointMappings intelligently maps API endpoints to Go methods.
func createEndpointMappings(endpoints []EndpointMapping, methods []GoMethod) []EndpointMapping {
	mappings := make([]EndpointMapping, len(endpoints))
	copy(mappings, endpoints)

	// Try to match each endpoint with a Go method
	for i := range mappings {
		goMethod := matchMethodToEndpoint(mappings[i], methods)
		if goMethod != nil {
			mappings[i].GoMethod = goMethod
			mappings[i].IsImplemented = true
		}
	}

	return mappings
}

// determinePriority assigns priority based on endpoint characteristics.
// Tweak this logic as needed to fit project goals.
func determinePriority(mapping EndpointMapping) string {
	path := mapping.APIEndpoint

	// Low priority: Admin and advanced features (check first)
	if contains(path, "admin") || contains(path, "settings") {
		return "Low"
	}

	// High priority: Core functionality
	if contains(path, "/v2/translate") && mapping.HTTPMethod == "POST" {
		return "High"
	}
	if contains(path, "/v2/languages") && mapping.HTTPMethod == "GET" {
		return "High"
	}

	// Medium priority: Important features
	if contains(path, "/v2/usage") {
		return "Medium"
	}
	if contains(path, "/v2/") && (mapping.HTTPMethod == "GET" || mapping.HTTPMethod == "POST") {
		return "Medium"
	}

	// Default to Medium
	return "Medium"
}

// matchMethodToEndpoint attempts to find the best Go method for an API endpoint.
func matchMethodToEndpoint(endpoint EndpointMapping, methods []GoMethod) *GoMethod {
	// Strategy 1: Exact operation ID match
	if endpoint.OperationID != "" {
		for i := range methods {
			// Convert operation ID to Go method naming convention
			expectedName := operationIDToMethodName(endpoint.OperationID)
			if methods[i].Name == expectedName {
				return &methods[i]
			}
		}
	}

	// Strategy 2: Path-based matching
	for i := range methods {
		if pathMatchesMethod(endpoint.APIEndpoint, methods[i].Name) {
			return &methods[i]
		}
	}

	// No match found
	return nil
}

// Project setup
// ---------------------------

// ensureProjectRoot changes directory to project root if we're currently in "tools/".
func ensureProjectRoot() error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	// If we're in tools/ directory, move up one level
	if strings.HasSuffix(cwd, "/tools") || strings.HasSuffix(cwd, "\\tools") {
		parentDir := filepath.Dir(cwd)
		if err := os.Chdir(parentDir); err != nil {
			return fmt.Errorf("failed to change to parent directory: %w", err)
		}

		fmt.Printf("📂 Changed working directory to project root: %s\n", parentDir)
	}

	return nil
}

// Main function
// ----------------------------------------------------------------------------

func main() {
	// Ensure we're running from the project root
	if err := ensureProjectRoot(); err != nil {
		fmt.Printf("❌ Failed to ensure project root: %v\n", err)

		os.Exit(1)
	}

	analyzer := NewCoverageAnalyzer()

	if err := analyzer.Run(sourceCodeRoot, coverageReportFilePath); err != nil {
		fmt.Printf("❌ Analysis failed: %v\n", err)

		os.Exit(1)
	}
}

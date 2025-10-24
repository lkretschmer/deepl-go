// Package main generates API coverage analysis for the deepl-go library.
//
// This tool compares the official DeepL API specification with the currently
// implemented Go methods to identify coverage gaps and prioritize future development.
//
// The generator performs the following operations:
// 1. Fetch the latest OpenAPI specification from DeepL's official repository
// 2. Parse and analyze the Go source code to detect implemented client methods
// 3. Create intelligent mappings between API endpoints and Go methods
// 4. Generate comprehensive coverage reports in Markdown format
//
// Generated files:
// - openapi_spec.yaml: Official DeepL OpenAPI specification (cached locally)
// - api_coverage.md: Detailed coverage analysis and implementation status
//
// Security considerations:
// - HTTP requests are made to trusted sources (github.com/DeepLcom)
// - File operations are restricted to the testdata directory
// - No sensitive data is processed or stored

package main

// Tool dependencies for code generation and analysis
import (
	_ "gopkg.in/yaml.v3" // Required for OpenAPI specification parsing in testdata/gen_api_coverage.go
)

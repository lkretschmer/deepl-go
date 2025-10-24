# Development Tools

This directory contains development tools for the deepl-go library.

## API Coverage Analysis Tool

### Overview

This tool downloads the official DeepL API OpenAPI specification and analyzes the current Go client implementation to generate a detailed API coverage report.

### Files

- **`gen_api_coverage.go`** - Main generator tool for API coverage analysis
- **`gen_api_coverage_test.go`** - Comprehensive test suite
- **`tools.go`** - Go module dependency management
- **`README.md`** - This documentation

### Generated Files

- **`../api_coverage_report.md`** - Comprehensive API coverage report
- **`testdata/openapi_spec.yaml`** - Cached copy of official DeepL OpenAPI specification

## Usage

### Generate Coverage Report

```bash
# From project root
go generate ./tools

# Or run directly
go run ./tools/gen_api_coverage.go
```

### Report Structure

The generated `api_coverage_report.md` includes:

- **Executive Summary**: Overall coverage statistics
- **Coverage by Category**: Breakdown by API categories
- **Implemented Endpoints**: Currently working Go methods with API mappings
- **Missing Endpoints**: Prioritized list of unimplemented features
- **Go Client Methods**: Detailed list of detected methods
- **Recommendations**: Implementation priorities

## How It Works

1. **OpenAPI Specification Fetching**: Downloads latest spec from DeepL's GitHub repository with caching
2. **Go Source Code Analysis**: Uses AST parsing to detect client methods automatically
3. **Intelligent Endpoint Mapping**: Maps API endpoints to Go methods using operation ID and path matching
4. **Report Generation**: Creates comprehensive Markdown report with actionable insights

## Current Status

As of the latest analysis:

- **Total API Endpoints**: 25 official DeepL API endpoints
- **Implemented Endpoints**: 5 endpoints (20% coverage)
- **Go Client Methods**: 12 detected methods
- **High Priority Missing**: Document translation, glossary management
- **Categories**: Full coverage for translation/languages/usage, gaps in utilities/administration

## Contributing

When adding new API endpoint implementations:

1. Implement the new client method following existing patterns
2. Add appropriate tests and documentation
3. Run `go generate ./tools` to update the coverage report
4. Include the updated report in your pull request
5. Verify coverage percentage improvement in the executive summary

The coverage analysis helps maintain high-quality API compatibility and guides strategic development decisions.

package deepl

// Sample Go client code for testing AST analysis
// This file simulates the deepl-go library's Client methods

import (
	"context"
)

// Client represents the DeepL API client
type Client struct {
	apiKey string
}

// TranslateText translates text from source to target language
func (c *Client) TranslateText(text string, opts *TranslateOptions) (*TranslateResponse, error) {
	return &TranslateResponse{Text: "translated"}, nil
}

// TranslateTextWithContext translates text with context
func (c *Client) TranslateTextWithContext(ctx context.Context, text string, opts *TranslateOptions) (*TranslateResponse, error) {
	return &TranslateResponse{Text: "translated"}, nil
}

// GetLanguages retrieves supported languages
func (c *Client) GetLanguages() ([]Language, error) {
	return []Language{{Code: "EN", Name: "English"}}, nil
}

// GetLanguagesWithContext retrieves supported languages with context
func (c *Client) GetLanguagesWithContext(ctx context.Context) ([]Language, error) {
	return []Language{{Code: "EN", Name: "English"}}, nil
}

// GetTargetLanguages retrieves target languages
func (c *Client) GetTargetLanguages() ([]Language, error) {
	return []Language{{Code: "DE", Name: "German"}}, nil
}

// GetSourceLanguages retrieves source languages
func (c *Client) GetSourceLanguages() ([]Language, error) {
	return []Language{{Code: "EN", Name: "English"}}, nil
}

// Rephrase rephrases the given text
func (c *Client) Rephrase(text string) (*RephraseResponse, error) {
	return &RephraseResponse{Text: "rephrased"}, nil
}

// RephraseWithContext rephrases with context
func (c *Client) RephraseWithContext(ctx context.Context, text string) (*RephraseResponse, error) {
	return &RephraseResponse{Text: "rephrased"}, nil
}

// RephraseWithOptions rephrases with options
func (c *Client) RephraseWithOptions(text string, opts *RephraseOptions) (*RephraseResponse, error) {
	return &RephraseResponse{Text: "rephrased"}, nil
}

// GetUsage retrieves API usage information
func (c *Client) GetUsage() (*Usage, error) {
	return &Usage{CharacterCount: 1000, CharacterLimit: 500000}, nil
}

// GetUsageWithContext retrieves usage with context
func (c *Client) GetUsageWithContext(ctx context.Context) (*Usage, error) {
	return &Usage{CharacterCount: 1000, CharacterLimit: 500000}, nil
}

// NonClientFunction is not a client method (should be ignored)
func NonClientFunction() {
	// This should not be detected as a client method
}

// Data structures
type TranslateOptions struct {
	SourceLang string
	TargetLang string
}

type TranslateResponse struct {
	Text string
}

type Language struct {
	Code string
	Name string
}

type RephraseOptions struct {
	Style string
}

type RephraseResponse struct {
	Text string
}

type Usage struct {
	CharacterCount int
	CharacterLimit int
}

// Package extractor provides interfaces and implementations for text extraction from documents.
package extractor

import (
	"context"
	"io"
)

// Extractor extracts text content from documents.
type Extractor interface {
	// Extract reads a document and returns its text content.
	// The filename and mimeType help determine the appropriate extraction method.
	Extract(ctx context.Context, file io.Reader, filename string, mimeType string) (*ExtractionResult, error)

	// SupportedFormats returns the file extensions this extractor can handle.
	SupportedFormats() []string
}

// ExtractionResult contains the extracted text and metadata.
type ExtractionResult struct {
	// Text is the extracted text content.
	Text string

	// PageCount is the number of pages (for PDFs and documents).
	PageCount int

	// Metadata contains additional extraction metadata.
	Metadata map[string]any
}

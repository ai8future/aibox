// Package embedder provides interfaces and implementations for text embedding.
package embedder

import "context"

// Embedder generates vector embeddings from text.
type Embedder interface {
	// Embed generates a vector embedding for a single text.
	Embed(ctx context.Context, text string) ([]float32, error)

	// EmbedBatch generates vector embeddings for multiple texts.
	// More efficient than calling Embed repeatedly for multiple texts.
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)

	// Dimensions returns the dimensionality of the embeddings.
	// For example, nomic-embed-text returns 768-dimensional vectors.
	Dimensions() int

	// Model returns the name of the embedding model being used.
	Model() string
}

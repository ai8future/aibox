// Package vectorstore provides interfaces and implementations for vector storage and search.
package vectorstore

import "context"

// Store is a vector database for storing and searching embeddings.
type Store interface {
	// CreateCollection creates a new collection with the specified dimensions.
	// The collection name should be unique per tenant/store combination.
	CreateCollection(ctx context.Context, name string, dimensions int) error

	// DeleteCollection removes a collection and all its points.
	DeleteCollection(ctx context.Context, name string) error

	// CollectionExists checks if a collection exists.
	CollectionExists(ctx context.Context, name string) (bool, error)

	// CollectionInfo returns metadata about a collection.
	CollectionInfo(ctx context.Context, name string) (*CollectionInfo, error)

	// Upsert adds or updates points in a collection.
	Upsert(ctx context.Context, collection string, points []Point) error

	// Search finds the most similar points to a query vector.
	Search(ctx context.Context, params SearchParams) ([]SearchResult, error)

	// Delete removes specific points from a collection by ID.
	Delete(ctx context.Context, collection string, ids []string) error
}

// Point represents a vector with its metadata.
type Point struct {
	// ID is the unique identifier for this point.
	ID string

	// Vector is the embedding vector.
	Vector []float32

	// Payload contains metadata about this point.
	// Common fields: tenant_id, thread_id, filename, chunk_index, text
	Payload map[string]any
}

// SearchParams contains parameters for a similarity search.
type SearchParams struct {
	// Collection is the name of the collection to search.
	Collection string

	// Vector is the query vector to find similar points for.
	Vector []float32

	// Limit is the maximum number of results to return.
	Limit int

	// Filter optionally restricts results to points matching conditions.
	Filter *Filter

	// ScoreThreshold optionally filters results below this similarity score.
	ScoreThreshold float32
}

// Filter restricts search results based on payload fields.
type Filter struct {
	// Must contains conditions that must all be true.
	Must []Condition
}

// Condition is a single filter condition.
type Condition struct {
	// Field is the payload field to filter on.
	Field string

	// Match is the value to match (exact match).
	Match any
}

// SearchResult is a single search result.
type SearchResult struct {
	// ID is the point's unique identifier.
	ID string

	// Score is the similarity score (higher = more similar).
	Score float32

	// Payload contains the point's metadata.
	Payload map[string]any
}

// CollectionInfo contains metadata about a collection.
type CollectionInfo struct {
	// Name is the collection name.
	Name string

	// PointCount is the number of points in the collection.
	PointCount int64

	// Dimensions is the vector dimensionality.
	Dimensions int
}

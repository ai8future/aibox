// Package openai provides the OpenAI LLM provider implementation.
package openai

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"

	"github.com/ai8future/airborne/internal/validation"
)

const (
	vectorStorePollingInterval = 2 * time.Second
	vectorStorePollingTimeout  = 5 * time.Minute
)

// FileStoreConfig contains configuration for file store operations.
type FileStoreConfig struct {
	APIKey  string
	BaseURL string

	// ExpirationDays is the number of days until the vector store expires.
	// 0 means no automatic expiration.
	ExpirationDays int
}

// FileStoreResult contains the result of a file store operation.
type FileStoreResult struct {
	StoreID   string
	Name      string
	Status    string
	FileCount int
	CreatedAt time.Time
}

// UploadedFile contains information about an uploaded file.
type UploadedFile struct {
	FileID   string
	StoreID  string
	Filename string
	Status   string
}

// CreateVectorStore creates a new OpenAI vector store.
func CreateVectorStore(ctx context.Context, cfg FileStoreConfig, name string) (*FileStoreResult, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("API key is required")
	}

	opts := []option.RequestOption{
		option.WithAPIKey(cfg.APIKey),
	}
	if cfg.BaseURL != "" {
		if err := validation.ValidateProviderURL(cfg.BaseURL); err != nil {
			return nil, fmt.Errorf("invalid base URL: %w", err)
		}
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}

	client := openai.NewClient(opts...)

	params := openai.VectorStoreNewParams{
		Name: openai.String(name),
	}

	if cfg.ExpirationDays > 0 {
		params.ExpiresAfter = openai.VectorStoreNewParamsExpiresAfter{
			Days: int64(cfg.ExpirationDays),
		}
	}

	slog.Info("creating openai vector store", "name", name)

	store, err := client.VectorStores.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("create vector store: %w", err)
	}

	slog.Info("openai vector store created",
		"store_id", store.ID,
		"name", store.Name,
	)

	return &FileStoreResult{
		StoreID:   store.ID,
		Name:      store.Name,
		Status:    string(store.Status),
		FileCount: int(store.FileCounts.Total),
		CreatedAt: time.Unix(store.CreatedAt, 0),
	}, nil
}

// UploadFileToVectorStore uploads a file to an OpenAI vector store.
// It first uploads the file to OpenAI's Files API, then adds it to the vector store.
func UploadFileToVectorStore(ctx context.Context, cfg FileStoreConfig, storeID string, filename string, content io.Reader) (*UploadedFile, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("API key is required")
	}
	if strings.TrimSpace(storeID) == "" {
		return nil, fmt.Errorf("store ID is required")
	}

	opts := []option.RequestOption{
		option.WithAPIKey(cfg.APIKey),
	}
	if cfg.BaseURL != "" {
		if err := validation.ValidateProviderURL(cfg.BaseURL); err != nil {
			return nil, fmt.Errorf("invalid base URL: %w", err)
		}
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}

	client := openai.NewClient(opts...)

	// Step 1: Upload file to OpenAI Files API
	slog.Info("uploading file to openai", "filename", filename, "store_id", storeID)

	uploadedFile, err := client.Files.New(ctx, openai.FileNewParams{
		File:    content,
		Purpose: openai.FilePurposeAssistants,
	})
	if err != nil {
		return nil, fmt.Errorf("upload file: %w", err)
	}

	slog.Info("file uploaded to openai",
		"file_id", uploadedFile.ID,
		"filename", uploadedFile.Filename,
	)

	// Step 2: Add file to vector store
	vsFile, err := client.VectorStores.Files.New(ctx, storeID, openai.VectorStoreFileNewParams{
		FileID: uploadedFile.ID,
	})
	if err != nil {
		return nil, fmt.Errorf("add file to vector store: %w", err)
	}

	slog.Info("file added to vector store",
		"file_id", uploadedFile.ID,
		"store_id", storeID,
		"status", vsFile.Status,
	)

	// Step 3: Poll until file is processed
	finalStatus, err := waitForFileProcessing(ctx, client, storeID, vsFile.ID)
	if err != nil {
		slog.Warn("file processing incomplete",
			"file_id", uploadedFile.ID,
			"store_id", storeID,
			"error", err,
		)
		// Return result anyway with current status
		return &UploadedFile{
			FileID:   uploadedFile.ID,
			StoreID:  storeID,
			Filename: uploadedFile.Filename,
			Status:   string(vsFile.Status),
		}, nil
	}

	return &UploadedFile{
		FileID:   uploadedFile.ID,
		StoreID:  storeID,
		Filename: uploadedFile.Filename,
		Status:   finalStatus,
	}, nil
}

// waitForFileProcessing polls until the file is processed or timeout.
func waitForFileProcessing(ctx context.Context, client openai.Client, storeID, vsFileID string) (string, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, vectorStorePollingTimeout)
	defer cancel()

	ticker := time.NewTicker(vectorStorePollingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-timeoutCtx.Done():
			return "in_progress", fmt.Errorf("file processing timeout")
		case <-ticker.C:
			vsFile, err := client.VectorStores.Files.Get(timeoutCtx, storeID, vsFileID)
			if err != nil {
				return "unknown", fmt.Errorf("get file status: %w", err)
			}

			switch vsFile.Status {
			case openai.VectorStoreFileStatusCompleted:
				return "completed", nil
			case openai.VectorStoreFileStatusFailed:
				errMsg := "unknown error"
				if vsFile.LastError.Code != "" {
					errMsg = vsFile.LastError.Message
				}
				return "failed", fmt.Errorf("file processing failed: %s", errMsg)
			case openai.VectorStoreFileStatusCancelled:
				return "cancelled", fmt.Errorf("file processing cancelled")
			case openai.VectorStoreFileStatusInProgress:
				// Continue polling
				continue
			default:
				// Unknown status, continue polling
				continue
			}
		}
	}
}

// DeleteVectorStore deletes an OpenAI vector store.
func DeleteVectorStore(ctx context.Context, cfg FileStoreConfig, storeID string) error {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return fmt.Errorf("API key is required")
	}
	if strings.TrimSpace(storeID) == "" {
		return fmt.Errorf("store ID is required")
	}

	opts := []option.RequestOption{
		option.WithAPIKey(cfg.APIKey),
	}
	if cfg.BaseURL != "" {
		if err := validation.ValidateProviderURL(cfg.BaseURL); err != nil {
			return fmt.Errorf("invalid base URL: %w", err)
		}
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}

	client := openai.NewClient(opts...)

	slog.Info("deleting openai vector store", "store_id", storeID)

	_, err := client.VectorStores.Delete(ctx, storeID)
	if err != nil {
		return fmt.Errorf("delete vector store: %w", err)
	}

	slog.Info("openai vector store deleted", "store_id", storeID)
	return nil
}

// GetVectorStore retrieves information about an OpenAI vector store.
func GetVectorStore(ctx context.Context, cfg FileStoreConfig, storeID string) (*FileStoreResult, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("API key is required")
	}
	if strings.TrimSpace(storeID) == "" {
		return nil, fmt.Errorf("store ID is required")
	}

	opts := []option.RequestOption{
		option.WithAPIKey(cfg.APIKey),
	}
	if cfg.BaseURL != "" {
		if err := validation.ValidateProviderURL(cfg.BaseURL); err != nil {
			return nil, fmt.Errorf("invalid base URL: %w", err)
		}
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}

	client := openai.NewClient(opts...)

	store, err := client.VectorStores.Get(ctx, storeID)
	if err != nil {
		return nil, fmt.Errorf("get vector store: %w", err)
	}

	return &FileStoreResult{
		StoreID:   store.ID,
		Name:      store.Name,
		Status:    string(store.Status),
		FileCount: int(store.FileCounts.Total),
		CreatedAt: time.Unix(store.CreatedAt, 0),
	}, nil
}

// ListVectorStores lists all vector stores for the account.
func ListVectorStores(ctx context.Context, cfg FileStoreConfig, limit int) ([]FileStoreResult, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("API key is required")
	}

	opts := []option.RequestOption{
		option.WithAPIKey(cfg.APIKey),
	}
	if cfg.BaseURL != "" {
		if err := validation.ValidateProviderURL(cfg.BaseURL); err != nil {
			return nil, fmt.Errorf("invalid base URL: %w", err)
		}
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}

	client := openai.NewClient(opts...)

	params := openai.VectorStoreListParams{}
	if limit > 0 {
		params.Limit = openai.Int(int64(limit))
	}

	page, err := client.VectorStores.List(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list vector stores: %w", err)
	}

	var results []FileStoreResult
	for _, store := range page.Data {
		results = append(results, FileStoreResult{
			StoreID:   store.ID,
			Name:      store.Name,
			Status:    string(store.Status),
			FileCount: int(store.FileCounts.Total),
			CreatedAt: time.Unix(store.CreatedAt, 0),
		})
	}

	return results, nil
}

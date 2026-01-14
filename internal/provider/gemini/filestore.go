// Package gemini provides the Google Gemini LLM provider implementation.
package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/ai8future/airborne/internal/validation"
)

const (
	fileSearchBaseURL         = "https://generativelanguage.googleapis.com/v1beta"
	fileSearchPollingInterval = 2 * time.Second
	fileSearchPollingTimeout  = 5 * time.Minute
)

// FileStoreConfig contains configuration for Gemini file store operations.
type FileStoreConfig struct {
	APIKey  string
	BaseURL string // Optional override for testing
}

// FileStoreResult contains the result of a file store operation.
type FileStoreResult struct {
	StoreID       string
	Name          string
	Status        string
	DocumentCount int
	CreatedAt     time.Time
}

// UploadedFile contains information about an uploaded file.
type UploadedFile struct {
	FileID    string
	StoreID   string
	Filename  string
	Status    string
	Operation string
}

// fileSearchStoreResponse represents the API response for a FileSearchStore.
type fileSearchStoreResponse struct {
	Name                   string `json:"name"`
	DisplayName            string `json:"displayName"`
	CreateTime             string `json:"createTime"`
	UpdateTime             string `json:"updateTime"`
	TotalDocumentCount     int    `json:"totalDocumentCount"`
	ProcessedDocumentCount int    `json:"processedDocumentCount"`
	FailedDocumentCount    int    `json:"failedDocumentCount"`
	SizeBytes              string `json:"sizeBytes"`
}

// operationResponse represents a long-running operation response.
type operationResponse struct {
	Name     string                 `json:"name"`
	Done     bool                   `json:"done"`
	Error    *operationError        `json:"error,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
	Response map[string]interface{} `json:"response,omitempty"`
}

// operationError represents an error from an operation.
type operationError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// getBaseURL returns the base URL for the FileSearch API.
func (cfg FileStoreConfig) getBaseURL() string {
	if cfg.BaseURL != "" {
		return cfg.BaseURL
	}
	return fileSearchBaseURL
}

// CreateFileSearchStore creates a new Gemini FileSearchStore.
func CreateFileSearchStore(ctx context.Context, cfg FileStoreConfig, name string) (*FileStoreResult, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("API key is required")
	}

	if cfg.BaseURL != "" {
		if err := validation.ValidateProviderURL(cfg.BaseURL); err != nil {
			return nil, fmt.Errorf("invalid base URL: %w", err)
		}
	}

	url := fmt.Sprintf("%s/fileSearchStores?key=%s", cfg.getBaseURL(), cfg.APIKey)

	reqBody := map[string]string{}
	if name != "" {
		reqBody["displayName"] = name
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	slog.Info("creating gemini file search store", "name", name)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("create file search store failed: %s - %s", resp.Status, string(body))
	}

	var storeResp fileSearchStoreResponse
	if err := json.NewDecoder(resp.Body).Decode(&storeResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	// Extract store ID from name (format: fileSearchStores/xxx)
	storeID := storeResp.Name
	if idx := strings.LastIndex(storeResp.Name, "/"); idx != -1 {
		storeID = storeResp.Name[idx+1:]
	}

	slog.Info("gemini file search store created",
		"store_id", storeID,
		"name", storeResp.DisplayName,
	)

	createdAt, _ := time.Parse(time.RFC3339, storeResp.CreateTime)

	return &FileStoreResult{
		StoreID:       storeID,
		Name:          storeResp.DisplayName,
		Status:        "ready",
		DocumentCount: storeResp.TotalDocumentCount,
		CreatedAt:     createdAt,
	}, nil
}

// UploadFileToFileSearchStore uploads a file to a Gemini FileSearchStore.
func UploadFileToFileSearchStore(ctx context.Context, cfg FileStoreConfig, storeID string, filename string, mimeType string, content io.Reader) (*UploadedFile, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("API key is required")
	}
	if strings.TrimSpace(storeID) == "" {
		return nil, fmt.Errorf("store ID is required")
	}

	if cfg.BaseURL != "" {
		if err := validation.ValidateProviderURL(cfg.BaseURL); err != nil {
			return nil, fmt.Errorf("invalid base URL: %w", err)
		}
	}

	// Read the file content
	fileContent, err := io.ReadAll(content)
	if err != nil {
		return nil, fmt.Errorf("read file content: %w", err)
	}

	// Use the upload endpoint with multipart
	baseURL := cfg.getBaseURL()
	// Replace /v1beta with /upload/v1beta for media upload
	if strings.Contains(baseURL, "/v1beta") {
		baseURL = strings.Replace(baseURL, "/v1beta", "/upload/v1beta", 1)
	} else {
		baseURL = strings.Replace(baseURL, fileSearchBaseURL, fileSearchBaseURL+"/upload", 1)
	}

	url := fmt.Sprintf("%s/fileSearchStores/%s:uploadToFileSearchStore?key=%s", baseURL, storeID, cfg.APIKey)

	slog.Info("uploading file to gemini file search store",
		"store_id", storeID,
		"filename", filename,
		"mime_type", mimeType,
	)

	// Create multipart request
	// For Gemini upload, we need to send metadata as JSON and file as binary
	// Using simple JSON metadata with file in body
	metadataURL := fmt.Sprintf("%s/fileSearchStores/%s:uploadToFileSearchStore?key=%s", cfg.getBaseURL(), storeID, cfg.APIKey)

	reqBody := map[string]interface{}{
		"displayName": filename,
		"mimeType":    mimeType,
	}
	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal metadata: %w", err)
	}

	// First, try the simple upload approach with metadata
	// Create a combined request body for the upload
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(fileContent))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	if mimeType != "" {
		req.Header.Set("Content-Type", mimeType)
	}
	req.Header.Set("X-Goog-Upload-Protocol", "raw")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute upload request: %w", err)
	}
	defer resp.Body.Close()

	// If raw upload fails, try JSON metadata approach
	if resp.StatusCode != http.StatusOK {
		// Try JSON metadata approach
		req2, err := http.NewRequestWithContext(ctx, http.MethodPost, metadataURL, bytes.NewReader(jsonBody))
		if err != nil {
			return nil, fmt.Errorf("create metadata request: %w", err)
		}
		req2.Header.Set("Content-Type", "application/json")

		resp2, err := http.DefaultClient.Do(req2)
		if err != nil {
			return nil, fmt.Errorf("execute metadata request: %w", err)
		}
		defer resp2.Body.Close()

		if resp2.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp2.Body)
			return nil, fmt.Errorf("upload to file search store failed: %s - %s", resp2.Status, string(body))
		}
		resp = resp2
	}

	var opResp operationResponse
	if err := json.NewDecoder(resp.Body).Decode(&opResp); err != nil {
		return nil, fmt.Errorf("decode operation response: %w", err)
	}

	slog.Info("file upload initiated",
		"store_id", storeID,
		"filename", filename,
		"operation", opResp.Name,
	)

	// Poll for completion
	status, err := waitForOperation(ctx, cfg, opResp.Name)
	if err != nil {
		slog.Warn("file processing incomplete",
			"store_id", storeID,
			"filename", filename,
			"error", err,
		)
	}

	// Extract file ID from operation response
	fileID := ""
	if opResp.Response != nil {
		if name, ok := opResp.Response["name"].(string); ok {
			if idx := strings.LastIndex(name, "/"); idx != -1 {
				fileID = name[idx+1:]
			} else {
				fileID = name
			}
		}
	}

	return &UploadedFile{
		FileID:    fileID,
		StoreID:   storeID,
		Filename:  filename,
		Status:    status,
		Operation: opResp.Name,
	}, nil
}

// waitForOperation polls until an operation completes.
func waitForOperation(ctx context.Context, cfg FileStoreConfig, operationName string) (string, error) {
	if operationName == "" {
		return "unknown", nil
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, fileSearchPollingTimeout)
	defer cancel()

	ticker := time.NewTicker(fileSearchPollingInterval)
	defer ticker.Stop()

	url := fmt.Sprintf("%s/%s?key=%s", cfg.getBaseURL(), operationName, cfg.APIKey)

	for {
		select {
		case <-timeoutCtx.Done():
			return "in_progress", fmt.Errorf("operation timeout")
		case <-ticker.C:
			req, err := http.NewRequestWithContext(timeoutCtx, http.MethodGet, url, nil)
			if err != nil {
				return "unknown", fmt.Errorf("create request: %w", err)
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return "unknown", fmt.Errorf("execute request: %w", err)
			}

			var opResp operationResponse
			if err := json.NewDecoder(resp.Body).Decode(&opResp); err != nil {
				resp.Body.Close()
				return "unknown", fmt.Errorf("decode response: %w", err)
			}
			resp.Body.Close()

			if opResp.Done {
				if opResp.Error != nil {
					return "failed", fmt.Errorf("operation failed: %s", opResp.Error.Message)
				}
				return "completed", nil
			}
		}
	}
}

// DeleteFileSearchStore deletes a Gemini FileSearchStore.
func DeleteFileSearchStore(ctx context.Context, cfg FileStoreConfig, storeID string, force bool) error {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return fmt.Errorf("API key is required")
	}
	if strings.TrimSpace(storeID) == "" {
		return fmt.Errorf("store ID is required")
	}

	if cfg.BaseURL != "" {
		if err := validation.ValidateProviderURL(cfg.BaseURL); err != nil {
			return fmt.Errorf("invalid base URL: %w", err)
		}
	}

	url := fmt.Sprintf("%s/fileSearchStores/%s?key=%s", cfg.getBaseURL(), storeID, cfg.APIKey)
	if force {
		url += "&force=true"
	}

	slog.Info("deleting gemini file search store", "store_id", storeID)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete file search store failed: %s - %s", resp.Status, string(body))
	}

	slog.Info("gemini file search store deleted", "store_id", storeID)
	return nil
}

// GetFileSearchStore retrieves information about a Gemini FileSearchStore.
func GetFileSearchStore(ctx context.Context, cfg FileStoreConfig, storeID string) (*FileStoreResult, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("API key is required")
	}
	if strings.TrimSpace(storeID) == "" {
		return nil, fmt.Errorf("store ID is required")
	}

	if cfg.BaseURL != "" {
		if err := validation.ValidateProviderURL(cfg.BaseURL); err != nil {
			return nil, fmt.Errorf("invalid base URL: %w", err)
		}
	}

	url := fmt.Sprintf("%s/fileSearchStores/%s?key=%s", cfg.getBaseURL(), storeID, cfg.APIKey)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get file search store failed: %s - %s", resp.Status, string(body))
	}

	var storeResp fileSearchStoreResponse
	if err := json.NewDecoder(resp.Body).Decode(&storeResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	// Determine status based on document counts
	status := "ready"
	if storeResp.ProcessedDocumentCount < storeResp.TotalDocumentCount {
		status = "processing"
	}
	if storeResp.FailedDocumentCount > 0 {
		status = "partial"
	}

	createdAt, _ := time.Parse(time.RFC3339, storeResp.CreateTime)

	return &FileStoreResult{
		StoreID:       storeID,
		Name:          storeResp.DisplayName,
		Status:        status,
		DocumentCount: storeResp.TotalDocumentCount,
		CreatedAt:     createdAt,
	}, nil
}

// ListFileSearchStores lists all FileSearchStores.
func ListFileSearchStores(ctx context.Context, cfg FileStoreConfig, limit int) ([]FileStoreResult, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("API key is required")
	}

	if cfg.BaseURL != "" {
		if err := validation.ValidateProviderURL(cfg.BaseURL); err != nil {
			return nil, fmt.Errorf("invalid base URL: %w", err)
		}
	}

	url := fmt.Sprintf("%s/fileSearchStores?key=%s", cfg.getBaseURL(), cfg.APIKey)
	if limit > 0 && limit <= 20 {
		url += fmt.Sprintf("&pageSize=%d", limit)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list file search stores failed: %s - %s", resp.Status, string(body))
	}

	var listResp struct {
		FileSearchStores []fileSearchStoreResponse `json:"fileSearchStores"`
		NextPageToken    string                    `json:"nextPageToken"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	var results []FileStoreResult
	for _, store := range listResp.FileSearchStores {
		storeID := store.Name
		if idx := strings.LastIndex(store.Name, "/"); idx != -1 {
			storeID = store.Name[idx+1:]
		}

		createdAt, _ := time.Parse(time.RFC3339, store.CreateTime)

		results = append(results, FileStoreResult{
			StoreID:       storeID,
			Name:          store.DisplayName,
			Status:        "ready",
			DocumentCount: store.TotalDocumentCount,
			CreatedAt:     createdAt,
		})
	}

	return results, nil
}

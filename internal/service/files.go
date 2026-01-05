package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	pb "github.com/cliffpyles/aibox/gen/go/aibox/v1"
	"github.com/cliffpyles/aibox/internal/rag"
)

// FileService implements the FileService gRPC service for RAG file management.
type FileService struct {
	pb.UnimplementedFileServiceServer

	ragService *rag.Service
}

// NewFileService creates a new file service.
func NewFileService(ragService *rag.Service) *FileService {
	return &FileService{
		ragService: ragService,
	}
}

// CreateFileStore creates a new vector store (Qdrant collection).
func (s *FileService) CreateFileStore(ctx context.Context, req *pb.CreateFileStoreRequest) (*pb.CreateFileStoreResponse, error) {
	if req.ClientId == "" {
		return nil, fmt.Errorf("client_id is required")
	}

	// Generate store ID if name is provided, otherwise use a UUID-like ID
	storeID := req.Name
	if storeID == "" {
		storeID = fmt.Sprintf("store_%d", time.Now().UnixNano())
	}

	// Create the Qdrant collection via RAG service
	if err := s.ragService.CreateStore(ctx, req.ClientId, storeID); err != nil {
		slog.Error("failed to create file store",
			"client_id", req.ClientId,
			"store_id", storeID,
			"error", err,
		)
		return nil, fmt.Errorf("create store: %w", err)
	}

	slog.Info("file store created",
		"client_id", req.ClientId,
		"store_id", storeID,
	)

	return &pb.CreateFileStoreResponse{
		StoreId:   storeID,
		Provider:  pb.Provider_PROVIDER_UNSPECIFIED, // We use self-hosted Qdrant
		Name:      req.Name,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// UploadFile uploads a file to a store using client streaming.
func (s *FileService) UploadFile(stream pb.FileService_UploadFileServer) error {
	ctx := stream.Context()

	// First message should be metadata
	firstMsg, err := stream.Recv()
	if err != nil {
		return fmt.Errorf("receive metadata: %w", err)
	}

	metadata := firstMsg.GetMetadata()
	if metadata == nil {
		return fmt.Errorf("first message must contain metadata")
	}

	if metadata.StoreId == "" {
		return fmt.Errorf("store_id is required")
	}
	if metadata.Filename == "" {
		return fmt.Errorf("filename is required")
	}

	slog.Info("starting file upload",
		"store_id", metadata.StoreId,
		"filename", metadata.Filename,
		"size", metadata.Size,
	)

	// Collect file chunks
	var buf bytes.Buffer
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("receive chunk: %w", err)
		}

		chunk := msg.GetChunk()
		if chunk == nil {
			continue
		}
		buf.Write(chunk)
	}

	// Extract tenant ID from context or use a default
	// In a real implementation, this would come from the auth interceptor
	tenantID := "default"

	// Ingest the file via RAG service
	result, err := s.ragService.Ingest(ctx, rag.IngestParams{
		StoreID:  metadata.StoreId,
		TenantID: tenantID,
		File:     &buf,
		Filename: metadata.Filename,
		MIMEType: metadata.MimeType,
	})
	if err != nil {
		slog.Error("failed to ingest file",
			"store_id", metadata.StoreId,
			"filename", metadata.Filename,
			"error", err,
		)
		return stream.SendAndClose(&pb.UploadFileResponse{
			FileId:   "",
			Filename: metadata.Filename,
			StoreId:  metadata.StoreId,
			Status:   "failed",
		})
	}

	slog.Info("file uploaded and indexed",
		"store_id", metadata.StoreId,
		"filename", metadata.Filename,
		"chunks", result.ChunkCount,
	)

	return stream.SendAndClose(&pb.UploadFileResponse{
		FileId:   fmt.Sprintf("%s_%s", metadata.StoreId, metadata.Filename),
		Filename: metadata.Filename,
		StoreId:  metadata.StoreId,
		Status:   "ready",
	})
}

// DeleteFileStore deletes a store and all its contents.
func (s *FileService) DeleteFileStore(ctx context.Context, req *pb.DeleteFileStoreRequest) (*pb.DeleteFileStoreResponse, error) {
	if req.StoreId == "" {
		return nil, fmt.Errorf("store_id is required")
	}

	// Extract tenant ID from context
	tenantID := "default"

	if err := s.ragService.DeleteStore(ctx, tenantID, req.StoreId); err != nil {
		slog.Error("failed to delete file store",
			"store_id", req.StoreId,
			"error", err,
		)
		return &pb.DeleteFileStoreResponse{
			Success: false,
			Message: err.Error(),
		}, nil
	}

	slog.Info("file store deleted", "store_id", req.StoreId)

	return &pb.DeleteFileStoreResponse{
		Success: true,
		Message: "store deleted successfully",
	}, nil
}

// GetFileStore retrieves store information.
func (s *FileService) GetFileStore(ctx context.Context, req *pb.GetFileStoreRequest) (*pb.GetFileStoreResponse, error) {
	if req.StoreId == "" {
		return nil, fmt.Errorf("store_id is required")
	}

	// Extract tenant ID from context
	tenantID := "default"

	info, err := s.ragService.StoreInfo(ctx, tenantID, req.StoreId)
	if err != nil {
		return nil, fmt.Errorf("get store info: %w", err)
	}

	if info == nil {
		return nil, fmt.Errorf("store not found")
	}

	return &pb.GetFileStoreResponse{
		StoreId:   req.StoreId,
		Name:      info.Name,
		Provider:  pb.Provider_PROVIDER_UNSPECIFIED,
		FileCount: int32(info.PointCount), // Each file may have multiple chunks
		Status:    "ready",
		CreatedAt: "", // Not tracked in Qdrant by default
	}, nil
}

// ListFileStores lists all stores for a client.
func (s *FileService) ListFileStores(ctx context.Context, req *pb.ListFileStoresRequest) (*pb.ListFileStoresResponse, error) {
	// For now, return empty list - would need to implement collection listing in Qdrant
	// This would require storing metadata about stores separately
	return &pb.ListFileStoresResponse{
		Stores: []*pb.FileStoreSummary{},
	}, nil
}

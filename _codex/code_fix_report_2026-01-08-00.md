# Code Fix Report
Date Created: 2026-01-08 00:05:43 +0100
Date Updated: 2026-01-08

## Overview
Reviewed tenant loading, auth interceptors, and RAG file ingestion paths for correctness in multi-tenant deployments and file upload workflows.

## Fixed in v0.5.7-0.5.13

### 1) Tenant ID normalization mismatch - FIXED (v0.5.12)
Tenant IDs are now normalized (lowercase, trimmed) on config load in `internal/tenant/loader.go`.

### 2) FileService blocked by tenant interceptor - FIXED (v0.5.10)
Added FileService methods to skipMethods in `internal/auth/tenant_interceptor.go`.

### 3) RAG ingestion point ID collisions - FIXED (v0.5.13)
Added unique FileID field to IngestParams. Each upload generates a random file ID (`file_<hex>`) for unique point IDs, preventing collisions when uploading files with the same name.

## All issues from this report have been fixed.

## Verification
- go test ./internal/tenant/...
- go test ./internal/rag/...
- go test ./internal/service/...
- go test ./...

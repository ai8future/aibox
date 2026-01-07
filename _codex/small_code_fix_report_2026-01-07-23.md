# Small Code Fix Report
Date Created: 2026-01-07 23:03:36 +0100
Date Updated: 2026-01-08

## Fixed in v0.5.7-0.5.13

### 1) Tenant IDs normalized on requests but stored raw - FIXED (v0.5.12)
Tenant IDs are now normalized (lowercase, trimmed) during config load in `internal/tenant/loader.go`.

### 2) Rate limiter accepts negative token counts - FIXED (v0.5.9)
Added `if tokens <= 0 { return nil }` check in `RecordTokens` function.

### 3) Logging config is loaded but not applied - FIXED (v0.5.8)
Added `configureLogger(cfg.Logging)` function that applies log level and format from config.

## All issues from this report have been fixed.

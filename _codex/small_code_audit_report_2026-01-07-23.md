# Small Code Audit Report
Date Created: 2026-01-07 23:03:36 +0100
Date Updated: 2026-01-08

## Scope
- Go services: config, auth, tenant, service layer, gRPC server, RAG, providers
- Focus: security posture + code quality issues with patch-ready fixes

## Fixed in v0.5.7-0.5.13

### 1) TLS env overrides are ignored - FIXED (v0.5.7)
Added AIBOX_TLS_ENABLED, AIBOX_TLS_CERT_FILE, AIBOX_TLS_KEY_FILE, REDIS_DB, AIBOX_LOG_FORMAT env overrides.

### 2) Tenant interceptor blocks FileService RPCs - FIXED (v0.5.10)
Added FileService methods to skipMethods in tenant interceptor.

### 3) SelectProvider lacks permission gate - FIXED (v0.5.11)
Added auth.RequirePermission(ctx, auth.PermissionChat) check.

## All issues from this report have been fixed.

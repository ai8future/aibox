# aibox Admin UI Design

**Date:** 2026-01-09
**Status:** Approved for implementation

## Overview

Web-based Admin UI for aibox providing API key management, tenant configuration, usage monitoring, and AI provider status. React frontend (adapted from bizops) with Go HTTP backend integrated into the existing aibox server.

## Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Frontend framework | React + Vite + Tailwind | Match bizops, copy components directly |
| Backend language | Go | Single binary, direct access to KeyStore/TenantManager |
| Admin auth | Password in local file | Works without Redis, simple reset |
| Session storage | In-memory | Don't need persistence across restarts |
| Deployment | Embedded in aibox binary | Single artifact, `go:embed` directive |

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                     aibox binary                         │
├─────────────────────┬───────────────────────────────────┤
│   gRPC Server       │         Admin HTTP Server         │
│   :50051            │         :8080                     │
│                     │                                   │
│   - Chat            │   ┌─────────────────────────────┐ │
│   - Files           │   │  Static Files (React SPA)  │ │
│   - Admin           │   └─────────────────────────────┘ │
│                     │   ┌─────────────────────────────┐ │
│                     │   │  /api/* REST endpoints     │ │
│                     │   │  - auth, keys, tenants     │ │
│                     │   │  - usage, providers        │ │
│                     │   └─────────────────────────────┘ │
├─────────────────────┴───────────────────────────────────┤
│                   Shared Services                        │
│   KeyStore │ TenantManager │ RateLimiter │ Redis        │
└─────────────────────────────────────────────────────────┘
```

## Admin Features

1. **Dashboard** - Health, stats, provider status summary
2. **API Keys** - CRUD with permissions and rate limits
3. **Tenants** - View/edit tenant configurations
4. **Usage** - Token/request charts with time ranges
5. **AI Providers** - OpenAI, Anthropic, Gemini status (from bizops)

## REST API

All endpoints under `/api/` prefix. Auth endpoints are public, all others require session token.

See implementation plan for full endpoint list.

## File Structure

```
admin/
└── frontend/           # React SPA
    ├── src/components/ # UI components
    ├── package.json
    └── vite.config.js

internal/
└── admin/              # Go handlers
    ├── server.go
    ├── auth.go
    └── handlers_*.go

configs/
└── .admin_credentials  # Bcrypt password hash (gitignored)
```

## Implementation Phases

1. Frontend setup (copy from bizops)
2. Go admin server + auth
3. API key management
4. Tenant & usage endpoints
5. Provider status
6. Static embedding + Makefile

## References

- bizops frontend: `/Users/cliff/Desktop/_code/bizops/admin/frontend/`
- Implementation plan: `/Users/cliff/.claude/plans/zesty-conjuring-milner.md`

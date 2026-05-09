# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this project is

A Go backend for a corporate resource booking system (ВКР/diploma thesis). Employees and admins can book meeting rooms and workspaces. The server enforces RBAC via JWT.

## Commands

```bash
# Run all tests (no external deps required — tests use in-memory repo)
go test ./...

# Run a single package's tests
go test ./internal/service/...
go test ./internal/http/...


# Run the server locally (requires a running Postgres)
go run .

# Docker stack (Postgres + Redis + app)
./booking up          # docker compose up -d
./booking down
./booking restart
./booking logs
./booking init-env    # generate .env from .env.example with random secrets
```

## Environment variables

All config is read from env by `internal/config/config.go`. Key variables:

| Variable | Default | Notes |
|---|---|---|
| `APP_ADDRESS` | `:8080` | listen address |
| `APP_DATABASE_URL` | postgres://postgres:postgres@localhost:5432/diplom | pgx connection string |
| `APP_JWT_SECRET` | `development-secret` | HS256 signing key |
| `APP_REDIS_ENABLED` | `false` | set `true` to enable Redis cache |
| `APP_REDIS_ADDR` | `localhost:6379` | |
| `APP_ADMIN_EMAIL` | `admin@corp.local` | seeded on startup |
| `APP_ADMIN_PASSWORD` | `admin123` | seeded on startup |

Copy `.env.example` to `.env` before running with Docker.

## Architecture

The application has four layers that depend strictly downward:

```
main → internal/http (App) → internal/service → internal/repository (interfaces)
                                                       ↓
                                           internal/repository/postgres (Store)
                                           internal/repository/memory (in tests)
```

**`internal/domain/models.go`** — all shared types (`User`, `Resource`, `Booking`, `UtilizationReportItem`, role/status constants). No logic here.

**`internal/repository/interfaces.go`** — `UserRepository`, `ResourceRepository`, `BookingRepository`, and the composite `Store` interface. The `memory.go` in the same package is an in-memory implementation used exclusively by tests.

**`internal/repository/postgres/store.go`** — the real `*Store`, implementing all three repository interfaces against PostgreSQL via `database/sql` + `pgx/v5/stdlib`. SQL migrations are embedded as `*.sql` files and run in alphabetical order on startup (`store.Migrate()`). Migrations are non-transactional, re-runnable `IF NOT EXISTS` DDL.

**`internal/service/services.go`** — `AuthService`, `ResourceService`, `BookingService`. Business rules live here (conflict detection, cancellation, slug normalisation for equipment tags, image URL validation). Cache is invalidated on every write by prefix (`availability:`, `utilization:`). Password hashing is SHA-256 hex (no bcrypt).

**`internal/service/recommendations.go`** — `BookingService.RecommendSchedule` scores time-slots using a weighted formula (0.5 × time-deviation + 0.3 × capacity-excess + 0.2 × recent-load) and returns the top 3 candidates. Also owns `UtilizationReport` with per-hour and per-weekday statistics.

**`internal/http/api.go`** — `App` wires config → store → services → routes → `net/http` server. Route-level middleware: `requireAuth` (Bearer JWT), `requireAdmin`, `requireAdminForMethods` (method-specific admin gate on the same path). All times are parsed/returned as RFC3339. `decodeJSON` disallows unknown fields.

**`internal/cache/`** — thin `Cache` interface (`Get/Set/DeleteByPrefix`). `noop.go` is the no-op default; `redis.go` is the Redis implementation. Services accept `cache.Cache` and use `loadCachedJSON`/`storeCachedJSON` helpers. TTL is 2 minutes.

## Key conventions

- Equipment tags are normalised to lowercase slugs (`slugify`) before storage and query; always compare/store slugs, not raw strings.
- `DELETE /resources/{id}` soft-disables (sets `is_active=false`) and auto-cancels all future active bookings for that resource; it does **not** delete the row.
- The `memory.go` in-memory store is **only** for tests — it is not used in production.
- All timestamps are stored and compared in UTC.
- `BookingService` and `ResourceService` both hold a `cache.Cache`; invalidation always removes both the `availability:` and `utilization:` key prefixes together.

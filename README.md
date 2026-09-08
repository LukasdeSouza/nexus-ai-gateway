# Nexus AI Gateway

**Nexus AI Gateway** is a high-performance, resilient, production-grade AI infrastructure gateway positioned between application backends and multiple AI model providers (OpenAI, Anthropic, Gemini).

It provides a single, stable, **OpenAI-compatible HTTP API** while centralizing multi-tenant isolation, BYOK (Bring Your Own Key) credential management, rate limiting, routing policies, provider fallback, usage metering, and observability.

---

## High-Level Architecture

```
                       CLIENT APPLICATIONS
                                |
                                v
                  +---------------------------+
                  |       API / DATA PLANE    |
                  |        gateway-api        |
                  +-------------+-------------+
                                |
           +--------------------+--------------------+
           |                    |                    |
           v                    v                    v
        Auth/Key             Router              Rate Limit
        (Redis)           (Multi-model)          (Sliding)
           |                    |                    |
           +--------------------+--------------------+
                                |
                   +------------v------------+
                   |    Provider Adapters    |
                   +-----+-----------+-------+
                         |           |       |
                         v           v       v
                      OpenAI     Anthropic Gemini
```

---

## Key Features

- **OpenAI-Compatible API**: Seamless drop-in replacement — point standard SDKs or curl requests to `/v1/chat/completions` and `/v1/models`.
- **Multi-Provider Support**: Built-in adapters for **OpenAI** (GPT-4o, GPT-4o-mini), **Anthropic** (Claude 3.5 Sonnet, Claude 3 Haiku), and **Google Gemini** (Gemini 1.5 Flash, Gemini 1.5 Pro).
- **Streaming (SSE)**: Full Server-Sent Events streaming support normalized across all providers.
- **BYOK (Bring Your Own Key)**: Tenants register and manage their own provider credentials securely.
- **Tenant Isolation**: Multi-tenant hierarchy: `Organization -> Project -> API Key`.
- **Rate Limiting**: Redis-backed sliding-window rate limiter with fail-open resilience.
- **Observability Built-in**:
  - Prometheus metrics (`/metrics`) tracking HTTP and provider latencies, token counters, estimated USD cost, and rate-limit rejections.
  - OpenTelemetry distributed tracing with OTLP exporter support (Jaeger).
  - Structured JSON logging with automatic secret and payload redaction.

---

## Quick Start (Local Development)

### 1. Start Infrastructure with Docker Compose

```bash
docker compose -f deploy/compose/docker-compose.yml up -d
```

This starts:
- **PostgreSQL 16**: `localhost:5432`
- **Redis 7**: `localhost:6379`
- **Jaeger UI**: `http://localhost:16686`
- **Prometheus**: `http://localhost:9090`
- **Grafana**: `http://localhost:3000` (admin / admin)
- **gateway-api**: `http://localhost:8080`

### 2. Run Directly with Go

```bash
# Copy sample configuration
cp .env.example .env

# Run database migrations and start gateway
go run ./cmd/gateway-api
```

---

## API Usage Examples

### 1. Create an Organization & Project

```bash
# Create Organization
curl -X POST http://localhost:8080/v1/organizations \
  -H "Content-Type: application/json" \
  -d '{"name": "Acme Corp"}'

# Create Project
curl -X POST http://localhost:8080/v1/projects \
  -H "Content-Type: application/json" \
  -d '{"organization_id": "org_...", "name": "Production", "environment": "production"}'
```

### 2. Create an API Key

```bash
curl -X POST http://localhost:8080/v1/api-keys \
  -H "Content-Type: application/json" \
  -d '{"project_id": "prj_...", "env": "live"}'
```
*Note: The plaintext key (`ngk_live_...`) is returned only once upon creation.*

### 3. Send an OpenAI-Compatible Chat Request

```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer ngk_live_..." \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o",
    "messages": [
      {"role": "user", "content": "Explain Kubernetes simply."}
    ]
  }'
```

### 4. Stream Response (SSE)

```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer ngk_live_..." \
  -H "Content-Type: application/json" \
  -d '{
    "model": "smart",
    "stream": true,
    "messages": [
      {"role": "user", "content": "Hello!"}
    ]
  }'
```

---

## Nexus CLI Usage

The gateway comes with a standalone CLI (`nexus-cli`) for administration, session management, and chat diagnostics:

```bash
# View all available commands
nexus-cli help

# 1. Login with your Gateway API key (persisted to ~/.nexus/credentials.json)
nexus-cli login -key ngk_live_... -project prj_...

# 2. Inspect session info
nexus-cli whoami

# 3. List available models & aliases
nexus-cli models

# 4. Send a prompt (uses saved credentials automatically)
nexus-cli chat -model smart -prompt "Explain Kafka simply"

# 5. View token usage & estimated cost dashboard
nexus-cli usage

# 6. Register a provider key (BYOK)
nexus-cli providers add -provider openai -key sk-...

# 7. Logout
nexus-cli logout
```

---

## Testing

```bash
# Run unit tests
go test -v ./internal/auth/... ./internal/ratelimit/... ./internal/provider/...

# Run all tests
go test -v -race ./...
```

---

## Roadmap

- [x] **Phase 1**: Gateway Core, Multi-tenant Auth, Provider Adapters (OpenAI, Anthropic, Gemini), PostgreSQL, Redis, Prometheus Metrics
- [ ] **Phase 2**: Routing Policies (Cheapest, Fastest, Balanced), Automatic Retries, Health-Aware Fallback & Circuit Breaker
- [ ] **Phase 3**: Kafka Event Bus, Asynchronous `usage-service`, Aggregated Cost Analytics
- [ ] **Phase 4**: Production OpenTelemetry + Helm Charts on Kubernetes (GCP / GKE)
- [ ] **Phase 5**: TypeScript & Python SDKs, Go CLI
- [ ] **Phase 6**: Terraform Modules, Enterprise RBAC & Billing

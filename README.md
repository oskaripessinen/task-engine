# Cloud-Native Distributed Task Processing System

A production-oriented distributed task execution platform written in Go.

This repository captures the target architecture and engineering goals for a
cloud-native task engine. Sections below describe the intended end-state;
implementation is in progress.

---

## Overview

The system is designed to implement an asynchronous job processing
architecture:

- API service receives and validates jobs
- Jobs are persisted and enqueued
- Worker services consume and process jobs concurrently
- Retry logic and dead-letter isolation ensure reliability
- Observability is built-in (metrics, logs, health checks)
- Fully containerized for cloud deployment

The goal of this project is to model real-world backend systems used in modern
cloud environments.

---

## MVP scope and success criteria

### MVP scope

- API supports `POST /jobs` and `GET /jobs/{id}`
- Jobs are persisted in PostgreSQL and enqueued to Redis
- Workers pull from the queue, process a stub task, and update job status
- Basic health endpoint is available

### Success criteria

- Submitting a job returns a job ID
- A worker transitions the job through `queued -> running -> completed`
- Job status is queryable via `GET /jobs/{id}`
- API and worker run locally with Docker Compose dependencies

### Explicitly out of scope for MVP

- Dead-letter queues and advanced retry policy
- Authentication and authorization
- Distributed tracing and dashboards

---

## Architecture (target)

```text
Client
  |
  v
API Service (stateless)
  |
  +--> PostgreSQL (source of truth)
  |
  +--> Redis (queue)
             |
             v
       Worker Pool (N)
```

---

## Cloud engineering concepts demonstrated (target)

### Horizontal scalability

- Stateless API layer
- Worker pool scaling
- Queue-based decoupling
- Container-first architecture

### Reliability patterns

- At-least-once delivery semantics
- Retry with exponential backoff
- Dead-letter queue isolation
- Graceful shutdown with in-flight job protection

### Observability by default

- Prometheus metrics endpoint
- Structured JSON logging
- Job latency metrics (p95, p99)
- Queue depth monitoring
- Error rate tracking
- Health check endpoints

### Failure handling

- Worker crash recovery
- Idempotent job processing
- Controlled retry thresholds
- Backpressure via queue buffering

---

## Core features (planned)

- Concurrent worker execution
- Configurable worker pool size
- Retry logic with capped attempts
- Dead-letter queue handling
- Job status lifecycle tracking
- Dockerized services
- Production-ready folder structure

---

## Tech stack (intended)

- Go (concurrency-focused runtime)
- Redis (message queue)
- PostgreSQL (persistent state)
- Docker and Docker Compose
- Prometheus (metrics)
- Optional: Grafana dashboard

---

## Job lifecycle (target)

1. Client submits job
2. API validates and persists job (status = queued)
3. Job ID pushed to Redis queue
4. Worker consumes job
5. Status updated to running
6. Processing occurs
7. On success -> status = completed
8. On failure -> retry or DLQ after max attempts

---

## Job data model (MVP)

### Fields

- `id` (UUID): globally unique job identifier
- `payload` (JSON): job input data
- `status` (text): current lifecycle state
- `attempts` (int): processing attempts counter
- `created_at` (timestamp): creation time
- `updated_at` (timestamp): last update time

### Status values

- `queued`
- `running`
- `completed`
- `failed`

---

## Project structure (target)

```text
cmd/
  api/        # API entrypoint
  worker/     # Worker entrypoint

internal/
  config/         # Configuration management
  db/             # PostgreSQL integration
  queue/          # Redis queue abstraction
  job/            # Job processing logic
  observability/  # Metrics and logging
  retry/          # Backoff and retry logic

deploy/
  docker/     # Dockerfiles
  compose/    # docker-compose.yml

scripts/
  loadtest/   # k6 test scripts
```

---

## Running locally (planned)

### Start the full stack (Docker Compose)

```bash
docker compose -f deploy/compose/docker-compose.yml up -d --build
```

Configuration defaults are in `deploy/compose/.env`.

Migrations run automatically via the `migrate` service. To re-run manually:

```bash
docker compose -f deploy/compose/docker-compose.yml run --rm migrate
```

### Run API locally (optional)

```bash
go run ./cmd/api
```

### Run worker locally (optional)

```bash
go run ./cmd/worker
```

### Scale workers

```bash
docker compose -f deploy/compose/docker-compose.yml up -d --scale worker=3
```

The queue allows horizontal scaling without modifying the API layer.

### Try a job

```bash
curl -s -X POST http://localhost:8080/jobs \
  -H "Content-Type: application/json" \
  -d '{"payload":{"type":"demo","value":123}}'
```

```bash
curl -s http://localhost:8080/jobs/<job-id>
```

---

## Observability (planned)

Metrics endpoint exposed at:

```text
API: http://localhost:8080/metrics
Worker: http://localhost:9091/metrics
```

Example tracked metrics:

- `jobs_processed_total`
- `job_processing_duration_seconds`
- `job_failures_total`
- `queue_depth`
- `worker_active_goroutines`

---

## Design decisions (intent)

- Redis chosen for lightweight queue abstraction
- PostgreSQL used as authoritative state store
- API remains stateless for scaling
- Workers handle retries to isolate failure domains
- Exponential backoff reduces thundering herd risk
- Structured logging for log aggregation systems

---

## Production considerations (target)

- Kubernetes-ready architecture
- Horizontal Pod Autoscaler compatible
- Health probes for liveness and readiness
- Environment-based configuration
- Graceful termination handling (SIGTERM)

---

## Future improvements

- Kubernetes manifests
- Horizontal Pod Autoscaling
- Distributed tracing (OpenTelemetry)
- Circuit breaker implementation
- Multi-region queue replication
- Serverless execution comparison (cold start analysis)

---

## Why this project

This project focuses on modeling real-world cloud-native backend architecture
rather than building a UI-centric application.

It demonstrates practical understanding of:

- Distributed systems patterns
- Failure isolation
- Scalable service design
- Observability-first engineering
- Production deployment thinking

---

## License

MIT

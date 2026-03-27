# Task Engine

Small distributed task processing system written in Go.

The API accepts jobs, stores them in PostgreSQL, pushes job IDs to Redis, and
workers consume the queue and update job status.

## What it does

- accepts jobs over HTTP
- lists recent jobs over HTTP
- stores job payloads and status in PostgreSQL
- uses Redis as the queue between API and workers
- retries temporary failures with backoff and sends final failures to a dead-letter queue
- runs workers concurrently
- exposes health and metrics endpoints

## Architecture

```text
Client
  -> API
  -> PostgreSQL
  -> Redis
  -> Worker(s)
```

## Job flow

1. Client sends a job to the API.
2. API stores the job with status `queued`.
3. API pushes the job ID to Redis.
4. Worker reads the job ID from Redis.
5. Worker marks the job as `running`.
6. Worker retries temporary failures with backoff.
7. Worker finishes the job as `completed` or sends it to the dead-letter queue after the last failed attempt.

## Run with Docker Compose

```bash
docker compose -f deploy/compose/docker-compose.yml up -d --build
```

Services:

- API: `http://localhost:8080`
- Health: `http://localhost:8080/health`
- API metrics: `http://localhost:8080/metrics`
- Worker metrics: `http://localhost:9091/metrics`

## Try it

Create a job:

```bash
curl -s -X POST http://localhost:8080/jobs \
  -H "Content-Type: application/json" \
  -d '{"payload":{"type":"demo","value":123}}'
```

Fetch a job:

```bash
curl -s http://localhost:8080/jobs/<job-id>
```

List recent jobs:

```bash
curl -s http://localhost:8080/jobs?limit=10
```

Retry example:

```bash
curl -s -X POST http://localhost:8080/jobs \
  -H "Content-Type: application/json" \
  -d '{"payload":{"type":"demo","fail_until_attempt":2,"duration_ms":250}}'
```

## Run without Docker

You need PostgreSQL and Redis running locally.

API:

```bash
go run ./cmd/api
```

Worker:

```bash
go run ./cmd/worker
```

Default config:

- `DB_DSN=postgres://task:task@localhost:5432/task?sslmode=disable`
- `REDIS_ADDR=localhost:6379`
- `API_PORT=8080`
- `WORKER_COUNT=4`
- `WORKER_METRICS_PORT=9091`
- `MAX_ATTEMPTS=3`
- `RETRY_BASE_DELAY=1s`
- `RETRY_MAX_DELAY=10s`

## Project structure

```text
cmd/
  api/
  worker/

internal/
  config/
  db/
  job/
  observability/
  queue/
  retry/

deploy/
  compose/
  docker/

migrations/
```

## Notes

- database schema is in `migrations/001_create_jobs.sql`
- Docker Compose runs the migration automatically
- payload supports `type`, `duration_ms`, `fail`, `fail_until_attempt`, and `error`

## License

MIT

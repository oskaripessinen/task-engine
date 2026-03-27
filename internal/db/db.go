package db

import (
	"context"
	"database/sql"
	"errors"

	_ "github.com/lib/pq"

	"github.com/oskaripessinen/task-engine/internal/job"
)

var ErrNotFound = errors.New("not found")

type Store struct {
	db *sql.DB
}

func New(dsn string) (*Store, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}

	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *Store) CreateJob(ctx context.Context, payload []byte) (job.Job, error) {
	var created job.Job
	created.Payload = payload
	created.Status = job.StatusQueued

	row := s.db.QueryRowContext(
		ctx,
		`INSERT INTO jobs (payload, status, attempts) VALUES ($1, $2, 0)
		 RETURNING id, status, attempts, created_at, updated_at`,
		payload,
		created.Status,
	)

	if err := row.Scan(&created.ID, &created.Status, &created.Attempts, &created.CreatedAt, &created.UpdatedAt); err != nil {
		return job.Job{}, err
	}

	return created, nil
}

func (s *Store) GetJob(ctx context.Context, id string) (job.Job, error) {
	var fetched job.Job
	row := s.db.QueryRowContext(
		ctx,
		`SELECT id, payload, status, attempts, created_at, updated_at
		 FROM jobs WHERE id = $1`,
		id,
	)

	if err := row.Scan(&fetched.ID, &fetched.Payload, &fetched.Status, &fetched.Attempts, &fetched.CreatedAt, &fetched.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return job.Job{}, ErrNotFound
		}
		return job.Job{}, err
	}

	return fetched, nil
}

func (s *Store) ListJobs(ctx context.Context, limit int) ([]job.Job, error) {
	if limit <= 0 {
		limit = 20
	}

	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, payload, status, attempts, created_at, updated_at
		 FROM jobs
		 ORDER BY created_at DESC
		 LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	jobs := make([]job.Job, 0, limit)
	for rows.Next() {
		var item job.Job
		if err := rows.Scan(&item.ID, &item.Payload, &item.Status, &item.Attempts, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		jobs = append(jobs, item)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return jobs, nil
}

func (s *Store) StartJob(ctx context.Context, id string) (job.Job, error) {
	row := s.db.QueryRowContext(
		ctx,
		`UPDATE jobs
		 SET status = $2, attempts = attempts + 1, updated_at = now()
		 WHERE id = $1
		 RETURNING id, payload, status, attempts, created_at, updated_at`,
		id,
		job.StatusRunning,
	)

	var started job.Job
	if err := row.Scan(&started.ID, &started.Payload, &started.Status, &started.Attempts, &started.CreatedAt, &started.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return job.Job{}, ErrNotFound
		}
		return job.Job{}, err
	}

	return started, nil
}

func (s *Store) RecoverRunningJobs(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`UPDATE jobs
		 SET status = $1, updated_at = now()
		 WHERE status = $2
		 RETURNING id`,
		job.StatusQueued,
		job.StatusRunning,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return ids, nil
}

func (s *Store) UpdateJobStatus(ctx context.Context, id string, status job.Status) error {
	result, err := s.db.ExecContext(
		ctx,
		`UPDATE jobs SET status = $2, updated_at = now() WHERE id = $1`,
		id,
		status,
	)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

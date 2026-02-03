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

func (s *Store) MarkJobRunning(ctx context.Context, id string) (int, error) {
	row := s.db.QueryRowContext(
		ctx,
		`UPDATE jobs
		 SET status = $2, attempts = attempts + 1, updated_at = now()
		 WHERE id = $1
		 RETURNING attempts`,
		id,
		job.StatusRunning,
	)

	var attempts int
	if err := row.Scan(&attempts); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, err
	}

	return attempts, nil
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

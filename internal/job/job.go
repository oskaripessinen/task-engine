package job

import (
	"encoding/json"
	"time"
)

type Status string

const (
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
)

type Job struct {
	ID        string          `json:"id"`
	Payload   json.RawMessage `json:"payload"`
	Status    Status          `json:"status"`
	Attempts  int             `json:"attempts"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

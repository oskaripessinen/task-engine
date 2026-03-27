package job

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const defaultProcessingDuration = 100 * time.Millisecond

type Payload struct {
	Type             string `json:"type"`
	DurationMS       int    `json:"duration_ms"`
	Fail             bool   `json:"fail"`
	FailUntilAttempt int    `json:"fail_until_attempt"`
	ErrorMessage     string `json:"error"`
}

type RetryableError struct {
	err error
}

func (e RetryableError) Error() string {
	return e.err.Error()
}

func (e RetryableError) Unwrap() error {
	return e.err
}

func IsRetryable(err error) bool {
	var target RetryableError
	return errors.As(err, &target)
}

func Process(ctx context.Context, raw json.RawMessage, attempt int) error {
	payload, err := DecodePayload(raw)
	if err != nil {
		return err
	}

	duration := defaultProcessingDuration
	if payload.DurationMS > 0 {
		duration = time.Duration(payload.DurationMS) * time.Millisecond
	}

	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
	}

	if payload.Fail || (payload.FailUntilAttempt > 0 && attempt <= payload.FailUntilAttempt) {
		return RetryableError{err: errors.New(errorMessage(payload))}
	}

	return nil
}

func DecodePayload(raw json.RawMessage) (Payload, error) {
	var payload Payload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return Payload{}, fmt.Errorf("decode payload: %w", err)
	}
	if payload.Type == "" {
		return Payload{}, errors.New("payload.type is required")
	}
	if payload.DurationMS < 0 {
		return Payload{}, errors.New("payload.duration_ms must be greater than or equal to 0")
	}
	if payload.FailUntilAttempt < 0 {
		return Payload{}, errors.New("payload.fail_until_attempt must be greater than or equal to 0")
	}
	return payload, nil
}

func errorMessage(payload Payload) string {
	if payload.ErrorMessage != "" {
		return payload.ErrorMessage
	}
	return fmt.Sprintf("%s job failed", payload.Type)
}

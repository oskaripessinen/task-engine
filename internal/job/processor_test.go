package job

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDecodePayload(t *testing.T) {
	payload, err := DecodePayload([]byte(`{"type":"demo","duration_ms":25}`))
	if err != nil {
		t.Fatalf("DecodePayload returned error: %v", err)
	}
	if payload.Type != "demo" {
		t.Fatalf("Type = %q, want demo", payload.Type)
	}
	if payload.DurationMS != 25 {
		t.Fatalf("DurationMS = %d, want 25", payload.DurationMS)
	}
}

func TestDecodePayloadRejectsInvalidInput(t *testing.T) {
	_, err := DecodePayload([]byte(`{"duration_ms":10}`))
	if err == nil {
		t.Fatal("expected error for missing type")
	}
}

func TestProcessReturnsRetryableError(t *testing.T) {
	err := Process(context.Background(), []byte(`{"type":"demo","fail_until_attempt":2}`), 1)
	if !IsRetryable(err) {
		t.Fatalf("expected retryable error, got %v", err)
	}
}

func TestProcessSucceedsAfterRetryWindow(t *testing.T) {
	err := Process(context.Background(), []byte(`{"type":"demo","fail_until_attempt":2}`), 3)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
}

func TestProcessHonorsContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := Process(ctx, []byte(`{"type":"demo","duration_ms":50}`), 1)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
	if IsRetryable(err) {
		t.Fatal("context cancellation must not be retryable")
	}
}

func TestProcessUsesConfiguredDelay(t *testing.T) {
	start := time.Now()
	err := Process(context.Background(), []byte(`{"type":"demo","duration_ms":20}`), 1)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if time.Since(start) < 20*time.Millisecond {
		t.Fatal("process returned too early")
	}
}

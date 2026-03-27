package retry

import (
	"testing"
	"time"
)

func TestBackoff(t *testing.T) {
	tests := []struct {
		name    string
		attempt int
		want    time.Duration
	}{
		{name: "first attempt", attempt: 1, want: time.Second},
		{name: "second attempt", attempt: 2, want: 2 * time.Second},
		{name: "third attempt", attempt: 3, want: 4 * time.Second},
		{name: "capped", attempt: 6, want: 10 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Backoff(tt.attempt, time.Second, 10*time.Second)
			if got != tt.want {
				t.Fatalf("Backoff(%d) = %v, want %v", tt.attempt, got, tt.want)
			}
		})
	}
}

package observability

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

type Logger struct {
	service string
	encoder *json.Encoder
	mu      sync.Mutex
}

func NewLogger(service string) *Logger {
	return &Logger{
		service: service,
		encoder: json.NewEncoder(os.Stdout),
	}
}

func (l *Logger) Info(message string, fields map[string]any) {
	l.log("info", message, fields)
}

func (l *Logger) Error(message string, err error, fields map[string]any) {
	if fields == nil {
		fields = map[string]any{}
	}
	if err != nil {
		fields["error"] = err.Error()
	}
	l.log("error", message, fields)
}

func (l *Logger) log(level string, message string, fields map[string]any) {
	entry := map[string]any{
		"ts":      time.Now().UTC().Format(time.RFC3339Nano),
		"level":   level,
		"service": l.service,
		"msg":     message,
	}

	for key, value := range fields {
		entry[key] = value
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	_ = l.encoder.Encode(entry)
}

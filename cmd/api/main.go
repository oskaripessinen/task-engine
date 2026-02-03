package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/oskaripessinen/task-engine/internal/config"
	"github.com/oskaripessinen/task-engine/internal/db"
	"github.com/oskaripessinen/task-engine/internal/observability"
	"github.com/oskaripessinen/task-engine/internal/queue"
)

func main() {
	logger := observability.NewLogger("api")
	metrics := observability.NewMetrics()

	cfg, err := config.Load()
	if err != nil {
		logger.Error("config error", err, nil)
		os.Exit(1)
	}

	store, err := db.New(cfg.DBDSN)
	if err != nil {
		logger.Error("db error", err, nil)
		os.Exit(1)
	}
	defer func() {
		if err := store.Close(); err != nil {
			logger.Error("db close error", err, nil)
		}
	}()

	queueClient, err := queue.New(cfg.RedisAddr)
	if err != nil {
		logger.Error("queue error", err, nil)
		os.Exit(1)
	}
	defer func() {
		if err := queueClient.Close(); err != nil {
			logger.Error("queue close error", err, nil)
		}
	}()

	apiServer := newAPIServer(store, queueClient, logger, metrics)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", apiServer.healthHandler)
	mux.HandleFunc("/jobs", apiServer.jobsHandler)
	mux.HandleFunc("/jobs/", apiServer.jobByIDHandler)
	mux.Handle("/metrics", metrics)

	addr := fmt.Sprintf(":%d", cfg.APIPort)
	httpServer := &http.Server{
		Addr:    addr,
		Handler: loggingMiddleware(logger, mux),
	}

	logger.Info("api listening", map[string]any{"addr": addr})
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server error", err, nil)
		os.Exit(1)
	}
}

type apiServer struct {
	store   *db.Store
	queue   *queue.Queue
	logger  *observability.Logger
	metrics *observability.Metrics
}

func newAPIServer(store *db.Store, queueClient *queue.Queue, logger *observability.Logger, metrics *observability.Metrics) *apiServer {
	return &apiServer{store: store, queue: queueClient, logger: logger, metrics: metrics}
}

func (s *apiServer) healthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *apiServer) jobsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req createJobRequest
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json payload")
		return
	}

	if !isPayloadValid(req.Payload) {
		writeError(w, http.StatusBadRequest, "payload is required")
		return
	}

	created, err := s.store.CreateJob(r.Context(), req.Payload)
	if err != nil {
		s.logger.Error("create job error", err, nil)
		writeError(w, http.StatusInternalServerError, "failed to create job")
		return
	}
	s.metrics.IncJobsCreated()

	if err := s.queue.Enqueue(r.Context(), created.ID); err != nil {
		s.logger.Error("enqueue job error", err, nil)
		writeError(w, http.StatusInternalServerError, "failed to enqueue job")
		return
	}
	s.metrics.IncJobsEnqueued()

	writeJSON(w, http.StatusCreated, createJobResponse{ID: created.ID})
}

func (s *apiServer) jobByIDHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/jobs/")
	if id == "" || strings.Contains(id, "/") {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}
	if _, err := uuid.Parse(id); err != nil {
		writeError(w, http.StatusBadRequest, "invalid job id")
		return
	}

	fetched, err := s.store.GetJob(r.Context(), id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusNotFound, "job not found")
			return
		}
		s.logger.Error("get job error", err, nil)
		writeError(w, http.StatusInternalServerError, "failed to fetch job")
		return
	}

	writeJSON(w, http.StatusOK, fetched)
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func loggingMiddleware(logger *observability.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		wrapped := &statusWriter{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(wrapped, r)

		duration := time.Since(start)
		logger.Info("request", map[string]any{
			"method":   r.Method,
			"path":     r.URL.Path,
			"status":   wrapped.status,
			"duration": duration.String(),
		})
	})
}

type createJobRequest struct {
	Payload json.RawMessage `json:"payload"`
}

type createJobResponse struct {
	ID string `json:"id"`
}

func isPayloadValid(payload json.RawMessage) bool {
	trimmed := bytes.TrimSpace(payload)
	if len(trimmed) == 0 {
		return false
	}
	if bytes.Equal(trimmed, []byte("null")) {
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}

type errorResponse struct {
	Error string `json:"error"`
}

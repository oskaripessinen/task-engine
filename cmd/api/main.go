package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		errCh <- httpServer.ListenAndServe()
	}()

	logger.Info("api listening", map[string]any{"addr": addr})

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server error", err, nil)
			os.Exit(1)
		}
	case <-ctx.Done():
		logger.Info("shutdown requested", nil)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("shutdown error", err, nil)
		os.Exit(1)
	}

	logger.Info("api stopped", nil)
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

	checkCtx, cancel := context.WithTimeout(r.Context(), time.Second)
	defer cancel()

	status := healthResponse{Status: "ok", Database: "ok", Queue: "ok"}
	httpStatus := http.StatusOK

	if err := s.store.Ping(checkCtx); err != nil {
		status.Status = "degraded"
		status.Database = err.Error()
		httpStatus = http.StatusServiceUnavailable
	}
	if err := s.queue.Ping(checkCtx); err != nil {
		status.Status = "degraded"
		status.Queue = err.Error()
		httpStatus = http.StatusServiceUnavailable
	}

	writeJSON(w, httpStatus, status)
}

func (s *apiServer) jobsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listJobsHandler(w, r)
	case http.MethodPost:
		s.createJobHandler(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *apiServer) createJobHandler(w http.ResponseWriter, r *http.Request) {
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

func (s *apiServer) listJobsHandler(w http.ResponseWriter, r *http.Request) {
	limit := 20
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 || parsed > 100 {
			writeError(w, http.StatusBadRequest, "limit must be between 1 and 100")
			return
		}
		limit = parsed
	}

	jobs, err := s.store.ListJobs(r.Context(), limit)
	if err != nil {
		s.logger.Error("list jobs error", err, nil)
		writeError(w, http.StatusInternalServerError, "failed to list jobs")
		return
	}

	writeJSON(w, http.StatusOK, jobs)
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

type healthResponse struct {
	Status   string `json:"status"`
	Database string `json:"database"`
	Queue    string `json:"queue"`
}

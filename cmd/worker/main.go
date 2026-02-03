package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/oskaripessinen/task-engine/internal/config"
	"github.com/oskaripessinen/task-engine/internal/db"
	"github.com/oskaripessinen/task-engine/internal/job"
	"github.com/oskaripessinen/task-engine/internal/observability"
	"github.com/oskaripessinen/task-engine/internal/queue"
)

const (
	dequeueTimeout    = 5 * time.Second
	processingLatency = 100 * time.Millisecond
)

func main() {
	logger := observability.NewLogger("worker")
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

	metrics.SetWorkerGoroutines(cfg.WorkerCount)
	go startMetricsServer(logger, metrics, cfg.WorkerMetricsPort)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger.Info("worker started", map[string]any{"workers": cfg.WorkerCount})

	var wg sync.WaitGroup
	for i := 0; i < cfg.WorkerCount; i++ {
		wg.Add(1)
		workerID := i + 1
		go func() {
			defer wg.Done()
			workerLoop(ctx, workerID, store, queueClient, logger, metrics)
		}()
	}

	<-ctx.Done()
	logger.Info("shutdown requested", nil)
	wg.Wait()
	logger.Info("worker stopped", nil)
}

func startMetricsServer(logger *observability.Logger, metrics *observability.Metrics, port int) {
	addr := fmt.Sprintf(":%d", port)
	server := &http.Server{
		Addr:    addr,
		Handler: metrics,
	}

	logger.Info("metrics listening", map[string]any{"addr": addr})
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("metrics server error", err, nil)
	}
}

func workerLoop(ctx context.Context, workerID int, store *db.Store, queueClient *queue.Queue, logger *observability.Logger, metrics *observability.Metrics) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		jobID, err := queueClient.Dequeue(ctx, dequeueTimeout)
		if err != nil {
			if errors.Is(err, redis.Nil) {
				continue
			}
			if errors.Is(err, context.Canceled) {
				return
			}
			logger.Error("dequeue error", err, map[string]any{"worker_id": workerID})
			continue
		}

		attempts, err := store.MarkJobRunning(ctx, jobID)
		if err != nil {
			if errors.Is(err, db.ErrNotFound) {
				logger.Error("job not found", err, map[string]any{"job_id": jobID})
				continue
			}
			logger.Error("mark running error", err, map[string]any{"job_id": jobID})
			continue
		}

		logger.Info("job started", map[string]any{"job_id": jobID, "attempts": attempts, "worker_id": workerID})
		start := time.Now()

		if err := processJob(ctx); err != nil {
			metrics.IncJobsFailed()
			if updateErr := store.UpdateJobStatus(ctx, jobID, job.StatusFailed); updateErr != nil {
				logger.Error("update failed status error", updateErr, map[string]any{"job_id": jobID})
			}
			logger.Error("job failed", err, map[string]any{"job_id": jobID, "worker_id": workerID})
			metrics.ObserveProcessingDuration(time.Since(start))
			continue
		}

		metrics.IncJobsProcessed()
		metrics.ObserveProcessingDuration(time.Since(start))
		if err := store.UpdateJobStatus(ctx, jobID, job.StatusCompleted); err != nil {
			logger.Error("update completed status error", err, map[string]any{"job_id": jobID})
			continue
		}
		logger.Info("job completed", map[string]any{"job_id": jobID, "worker_id": workerID})
	}
}

func processJob(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(processingLatency):
		return nil
	}
}

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
	"github.com/oskaripessinen/task-engine/internal/retry"
)

const (
	dequeueTimeout = 5 * time.Second
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

	if err := recoverRunningJobs(ctx, store, queueClient, logger); err != nil {
		logger.Error("recover running jobs error", err, nil)
		os.Exit(1)
	}

	logger.Info("worker started", map[string]any{"workers": cfg.WorkerCount})

	var wg sync.WaitGroup
	for i := 0; i < cfg.WorkerCount; i++ {
		wg.Add(1)
		workerID := i + 1
		go func() {
			defer wg.Done()
			workerLoop(ctx, workerID, cfg, store, queueClient, logger, metrics)
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

func workerLoop(ctx context.Context, workerID int, cfg config.Config, store *db.Store, queueClient *queue.Queue, logger *observability.Logger, metrics *observability.Metrics) {
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

		startedJob, err := store.StartJob(ctx, jobID)
		if err != nil {
			if errors.Is(err, db.ErrNotFound) {
				logger.Error("job not found", err, map[string]any{"job_id": jobID})
				continue
			}
			logger.Error("mark running error", err, map[string]any{"job_id": jobID})
			continue
		}

		logger.Info("job started", map[string]any{"job_id": jobID, "attempts": startedJob.Attempts, "worker_id": workerID})
		start := time.Now()

		if err := job.Process(ctx, startedJob.Payload, startedJob.Attempts); err != nil {
			metrics.ObserveProcessingDuration(time.Since(start))
			if err := handleJobFailure(ctx, cfg, startedJob, err, workerID, store, queueClient, logger, metrics); err != nil {
				if errors.Is(err, context.Canceled) {
					return
				}
				logger.Error("handle job failure error", err, map[string]any{"job_id": jobID, "worker_id": workerID})
			}
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

func handleJobFailure(ctx context.Context, cfg config.Config, failedJob job.Job, processErr error, workerID int, store *db.Store, queueClient *queue.Queue, logger *observability.Logger, metrics *observability.Metrics) error {
	fields := map[string]any{
		"job_id":    failedJob.ID,
		"attempts":  failedJob.Attempts,
		"worker_id": workerID,
	}

	if job.IsRetryable(processErr) && failedJob.Attempts < cfg.MaxAttempts {
		delay := retry.Backoff(failedJob.Attempts, cfg.RetryBaseDelay, cfg.RetryMaxDelay)
		fields["retry_in"] = delay.String()
		fields["error"] = processErr.Error()
		logger.Info("job retry scheduled", fields)

		timer := time.NewTimer(delay)
		defer timer.Stop()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}

		if err := queueClient.Enqueue(ctx, failedJob.ID); err != nil {
			return fmt.Errorf("requeue job: %w", err)
		}
		metrics.IncJobsRetried()

		if err := store.UpdateJobStatus(ctx, failedJob.ID, job.StatusQueued); err != nil {
			logger.Error("update queued status error", err, map[string]any{"job_id": failedJob.ID})
		}

		logger.Info("job requeued", fields)
		return nil
	}

	metrics.IncJobsFailed()
	if err := store.UpdateJobStatus(ctx, failedJob.ID, job.StatusFailed); err != nil {
		logger.Error("update failed status error", err, map[string]any{"job_id": failedJob.ID})
	}
	if err := queueClient.EnqueueDeadLetter(ctx, failedJob.ID); err != nil {
		return fmt.Errorf("enqueue dead letter: %w", err)
	}
	metrics.IncJobsDeadLettered()
	logger.Error("job failed", processErr, fields)
	return nil
}

func recoverRunningJobs(ctx context.Context, store *db.Store, queueClient *queue.Queue, logger *observability.Logger) error {
	ids, err := store.RecoverRunningJobs(ctx)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}

	for _, id := range ids {
		if err := queueClient.Enqueue(ctx, id); err != nil {
			return err
		}
	}

	logger.Info("recovered running jobs", map[string]any{"count": len(ids)})
	return nil
}

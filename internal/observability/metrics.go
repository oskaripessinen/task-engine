package observability

import (
	"fmt"
	"net/http"
	"sync/atomic"
	"time"
)

type Metrics struct {
	jobsCreated             uint64
	jobsEnqueued            uint64
	jobsProcessed           uint64
	jobsFailed              uint64
	jobsRetried             uint64
	jobsDeadLettered        uint64
	processingDurationSumNs uint64
	processingDurationCount uint64
	workerGoroutines        int64
}

func NewMetrics() *Metrics {
	return &Metrics{}
}

func (m *Metrics) IncJobsCreated() {
	atomic.AddUint64(&m.jobsCreated, 1)
}

func (m *Metrics) IncJobsEnqueued() {
	atomic.AddUint64(&m.jobsEnqueued, 1)
}

func (m *Metrics) IncJobsProcessed() {
	atomic.AddUint64(&m.jobsProcessed, 1)
}

func (m *Metrics) IncJobsFailed() {
	atomic.AddUint64(&m.jobsFailed, 1)
}

func (m *Metrics) IncJobsRetried() {
	atomic.AddUint64(&m.jobsRetried, 1)
}

func (m *Metrics) IncJobsDeadLettered() {
	atomic.AddUint64(&m.jobsDeadLettered, 1)
}

func (m *Metrics) ObserveProcessingDuration(duration time.Duration) {
	atomic.AddUint64(&m.processingDurationSumNs, uint64(duration.Nanoseconds()))
	atomic.AddUint64(&m.processingDurationCount, 1)
}

func (m *Metrics) SetWorkerGoroutines(count int) {
	atomic.StoreInt64(&m.workerGoroutines, int64(count))
}

func (m *Metrics) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")

	jobsCreated := atomic.LoadUint64(&m.jobsCreated)
	jobsEnqueued := atomic.LoadUint64(&m.jobsEnqueued)
	jobsProcessed := atomic.LoadUint64(&m.jobsProcessed)
	jobsFailed := atomic.LoadUint64(&m.jobsFailed)
	jobsRetried := atomic.LoadUint64(&m.jobsRetried)
	jobsDeadLettered := atomic.LoadUint64(&m.jobsDeadLettered)
	processingSum := atomic.LoadUint64(&m.processingDurationSumNs)
	processingCount := atomic.LoadUint64(&m.processingDurationCount)
	workerGoroutines := atomic.LoadInt64(&m.workerGoroutines)

	_, _ = fmt.Fprintf(w, "# TYPE jobs_created_total counter\n")
	_, _ = fmt.Fprintf(w, "jobs_created_total %d\n", jobsCreated)
	_, _ = fmt.Fprintf(w, "# TYPE jobs_enqueued_total counter\n")
	_, _ = fmt.Fprintf(w, "jobs_enqueued_total %d\n", jobsEnqueued)
	_, _ = fmt.Fprintf(w, "# TYPE jobs_processed_total counter\n")
	_, _ = fmt.Fprintf(w, "jobs_processed_total %d\n", jobsProcessed)
	_, _ = fmt.Fprintf(w, "# TYPE job_failures_total counter\n")
	_, _ = fmt.Fprintf(w, "job_failures_total %d\n", jobsFailed)
	_, _ = fmt.Fprintf(w, "# TYPE job_retries_total counter\n")
	_, _ = fmt.Fprintf(w, "job_retries_total %d\n", jobsRetried)
	_, _ = fmt.Fprintf(w, "# TYPE jobs_dead_lettered_total counter\n")
	_, _ = fmt.Fprintf(w, "jobs_dead_lettered_total %d\n", jobsDeadLettered)
	_, _ = fmt.Fprintf(w, "# TYPE job_processing_duration_seconds summary\n")
	_, _ = fmt.Fprintf(w, "job_processing_duration_seconds_sum %.6f\n", float64(processingSum)/1e9)
	_, _ = fmt.Fprintf(w, "job_processing_duration_seconds_count %d\n", processingCount)
	_, _ = fmt.Fprintf(w, "# TYPE worker_active_goroutines gauge\n")
	_, _ = fmt.Fprintf(w, "worker_active_goroutines %d\n", workerGoroutines)
}

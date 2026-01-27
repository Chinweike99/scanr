package review

import (
	"context"
	"errors"
	"fmt"
	"log"
	"scanr/internal/fs"
	"scanr/internal/worker"
	"sync"
	"sync/atomic"
	"time"
)

// Config holds pipeline configuration
type Config struct {
	MaxWorkers     int
	MaxQueueSize   int
	MaxRetries     int
	TimeoutPerFile time.Duration
	DeadLetterSize int
	EnableMetrics  bool
}

// DefaultConfig returns the default pipeline configuration
func DefaultConfig() Config {
	return Config{
		MaxWorkers:     4,
		MaxQueueSize:   100,
		MaxRetries:     2,
		TimeoutPerFile: 30 * time.Second,
		DeadLetterSize: 1000,
		EnableMetrics:  true,
	}
}

// Pipeline implements the review pipeline with worker pool
type pipeline struct {
	config     Config
	reviewer   Reviewer
	workerPool *worker.WorkerPool
	deadLetter *worker.DeadLetterQueue
	metrics    *metrics
	stopOnce   sync.Once
	isRunning  atomic.Bool
}

// metrics tracks pipeline performance metrics
type metrics struct {
	filesProcessed atomic.Int64
	filesFailed    atomic.Int64
	filesRetried   atomic.Int64
	totalIssues    atomic.Int64
	totalDuration  atomic.Int64
}

func NewPipeline(config Config, reviewer Reviewer) (Pipeline, error) {
	if reviewer == nil {
		return nil, errors.New("reviewer cannot be nil")
	}

	if config.MaxWorkers <= 0 {
		config.MaxWorkers = 4
	}
	if config.MaxQueueSize <= 0 {
		config.MaxQueueSize = config.MaxWorkers * 2
	}
	if config.TimeoutPerFile <= 0 {
		config.TimeoutPerFile = 30 * time.Second
	}

	wp, err := worker.NewWorkerPool(config.MaxWorkers, config.MaxQueueSize)
	if err != nil {
		return nil, fmt.Errorf("failed to create worker pool: %w", err)
	}

	dlq := worker.NewDeadLetterQueue(config.DeadLetterSize)

	return &pipeline{
		config:     config,
		reviewer:   reviewer,
		workerPool: wp,
		deadLetter: dlq,
		metrics:    &metrics{},
	}, nil
}

// Run executes the review pipeline on the given files
func (p *pipeline) Run(ctx context.Context, files []*fs.FileInfo) (*ReviewResult, error) {
	if !p.isRunning.CompareAndSwap(false, true) {
		return nil, errors.New("pipeline is already running")
	}
	defer p.isRunning.Store(false)

	startTime := time.Now()

	// Create context with timeout for entire pipeline
	pipelineCtx, cancel := context.WithTimeout(ctx, p.calculateTimeout(len(files)))
	defer cancel()

	// Start worker pool with wrapper to match WorkerFunc signature
	workerFunc := func(ctx context.Context, file *fs.FileInfo) (interface{}, error) {
		return p.reviewer.ReviewFile(ctx, file)
	}
	if err := p.workerPool.Start(pipelineCtx, workerFunc); err != nil {
		return nil, fmt.Errorf("failed to start worker pool: %w", err)
	}

	// Setup result collection
	resultChan := make(chan worker.TaskResult, len(files))
	done := make(chan struct{})

	var result ReviewResult
	var wg sync.WaitGroup

	// Start result collector
	wg.Add(1)
	go p.collectResults(pipelineCtx, &result, resultChan, &wg, done)

	// Submit tasks
	if err := p.submitTasks(pipelineCtx, files, resultChan); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to submit tasks: %w", err)
	}

	// Wait for all tasks to complete before collecting results
	p.workerPool.Stop()

	// Close the result channel so collectResults can finish reading
	close(resultChan)
	wg.Wait()

	// Process dead letters (retry logic)
	// Note: processDeadLetters cannot send on resultChan after it's closed,
	// so we collect dead letter results separately
	p.processDeadLetters(pipelineCtx)

	// Finalize result
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(startTime)
	result.TotalFiles = len(files)

	// Log summary
	p.logSummary(&result)

	return &result, nil
}

// Stop stops the pipeline gracefully
func (p *pipeline) Stop() error {
	var err error
	p.stopOnce.Do(func() {
		if p.workerPool != nil {
			err = p.workerPool.Stop()
		}
	})
	return err
}

// submitTasks submits all files for review
func (p *pipeline) submitTasks(ctx context.Context, files []*fs.FileInfo, resultChan chan<- worker.TaskResult) error {
	batchSize := p.config.MaxWorkers * 2
	for i := 0; i < len(files); i += batchSize {
		end := i + batchSize
		if end > len(files) {
			end = len(files)
		}

		batch := files[i:end]
		if err := p.workerPool.SubmitBatch(ctx, batch, resultChan); err != nil {
			return fmt.Errorf("failed to submit batch %d-%d: %w", i, end, err)
		}

		// Check for cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}
	return nil
}

// collectResults collects results from the worker pool
func (p *pipeline) collectResults(ctx context.Context, result *ReviewResult,
	resultChan <-chan worker.TaskResult, wg *sync.WaitGroup, done chan<- struct{}) {
	defer wg.Done()

	fileReviews := make([]FileReview, 0)
	var mu sync.Mutex

	for taskResult := range resultChan {
		select {
		case <-ctx.Done():
			return
		default:
			p.processTaskResult(ctx, taskResult, &mu, &fileReviews, result)
		}
	}

	// Store final results
	result.FileReviews = fileReviews
	close(done)
}

// processTaskResult processes a single task result
func (p *pipeline) processTaskResult(ctx context.Context, taskResult worker.TaskResult,
	mu *sync.Mutex, fileReviews *[]FileReview, result *ReviewResult) {

	mu.Lock()
	defer mu.Unlock()

	fileReview := FileReview{
		File: taskResult.File,
	}

	if taskResult.Error != nil {
		fileReview.Error = taskResult.Error.Error()
		p.metrics.filesFailed.Add(1)

		// Add to dead letter queue for retry if applicable
		if taskResult.Retry {
			p.deadLetter.Push(worker.Task{
				ID:     taskResult.TaskID,
				File:   taskResult.File,
				Result: nil, // Will be set when retrying
				Ctx:    ctx,
			}, taskResult.Error, 1)
			p.metrics.filesRetried.Add(1)
		}
	} else {
		issues := taskResult.Issues.([]Issue)
		fileReview.Issues = issues
		fileReview.Duration = 0 // Will be populated by reviewer if available
		result.ReviewedFiles++

		// Count issues by severity
		for _, issue := range issues {
			result.TotalIssues++
			p.metrics.totalIssues.Add(1)

			switch issue.Severity {
			case SeverityCritical:
				result.CriticalCount++
			case SeverityHigh:
				result.WarningCount++
			case SeverityInfo:
				result.InfoCount++
			}
		}
	}

	*fileReviews = append(*fileReviews, fileReview)
}

// processDeadLetters processes tasks in the dead letter queue
func (p *pipeline) processDeadLetters(ctx context.Context) {
	if p.config.MaxRetries <= 0 {
		return
	}

	for i := 0; i < p.config.MaxRetries; i++ {
		dl, ok := p.deadLetter.Pop()
		if !ok {
			break
		}

		// Retry the task
		retryCtx, cancel := context.WithTimeout(ctx, p.config.TimeoutPerFile)

		issues, err := p.reviewer.ReviewFile(retryCtx, dl.Task.File)
		cancel()

		// Process result directly without sending on closed channel
		if err == nil && issues != nil {
			// Successfully retried - update metrics
			p.metrics.filesRetried.Add(-1) // Remove from retry count
		} else if err != nil {
			// Still failing, keep in dead letter for next retry cycle
			p.deadLetter.Push(dl.Task, err, dl.Attempts+1)
		}
	}
}

// calculateTimeout calculates the total timeout based on number of files
func (p *pipeline) calculateTimeout(numFiles int) time.Duration {
	baseTimeout := p.config.TimeoutPerFile * time.Duration(numFiles)

	// Add buffer for pipeline overhead
	pipelineOverhead := 10 * time.Second
	if baseTimeout < 30*time.Second {
		return 30*time.Second + pipelineOverhead
	}

	// Cap at 10 minutes
	maxTimeout := 10 * time.Minute
	if baseTimeout > maxTimeout {
		return maxTimeout
	}

	return baseTimeout + pipelineOverhead
}

// logSummary logs a summary of the review
func (p *pipeline) logSummary(result *ReviewResult) {
	log.Printf("Review completed:")
	log.Printf("  Total files: %d", result.TotalFiles)
	log.Printf("  Reviewed files: %d", result.ReviewedFiles)
	log.Printf("  Total issues: %d", result.TotalIssues)
	log.Printf("    Critical: %d", result.CriticalCount)
	log.Printf("    Warnings: %d", result.WarningCount)
	log.Printf("    Info: %d", result.InfoCount)
	log.Printf("  Duration: %v", result.Duration)

	if p.config.EnableMetrics {
		stats := p.workerPool.Stats()
		log.Printf("  Worker pool stats:")
		log.Printf("    Active workers: %d", stats["active"])
		log.Printf("    Queue size: %d", stats["queue_size"])
		log.Printf("    Total tasks: %d", stats["total_tasks"])
		log.Printf("    Failed tasks: %d", stats["failed_tasks"])
		log.Printf("    Retried tasks: %d", stats["retried_tasks"])
	}

	if deadLetterCount := p.deadLetter.Size(); deadLetterCount > 0 {
		log.Printf("  Dead letters: %d", deadLetterCount)
	}
}

// GetMetrics returns pipeline metrics
func (p *pipeline) GetMetrics() map[string]int64 {
	return map[string]int64{
		"files_processed": p.metrics.filesProcessed.Load(),
		"files_failed":    p.metrics.filesFailed.Load(),
		"files_retried":   p.metrics.filesRetried.Load(),
		"total_issues":    p.metrics.totalIssues.Load(),
		"total_duration":  p.metrics.totalDuration.Load(),
	}
}

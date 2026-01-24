package worker

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrPoolStopped     = errors.New("worker pool stopped")
	ErrPoolBusy        = errors.New("worker pool is too busy")
	ErrInvalidCapacity = errors.New("invalid worker capacity")
)

// Task represents a review task to be processed
type Task struct {
	ID     int
	File   fs.FileInfo
	Result chan<- TaskResult
	Ctx    context.Context
}

// TaskResult represents the result of processing a task
type TaskResult struct {
	TaskID  int
	File    fs.FileInfo
	Issues  interface{}
	Error   error
	Retry   bool
	Skipped bool
}

// WorkerPool implements a bounded worker pool for review tasks
type WorkerPool struct {
	capacity      int
	taskQueue     chan Task
	stopChan      chan struct{}
	stopped       atomic.Bool
	wg            sync.WaitGroup
	mu            sync.RWMutex
	activeWorkers atomic.Int32
	totalTasks    atomic.Int64
	failedTasks   atomic.Int64
	retriedTasks  atomic.Int64
}

// WorkerFunc is the function that processes a task
type WorkerFunc func(ctx context.Context, file fs.FileInfo) (interface{}, error)

func NewWorkerPool(capacity int, queueSize int) (*WorkerPool, error) {
	if capacity <= 0 {
		return nil, ErrInvalidCapacity
	}
	if queueSize < 0 {
		queueSize = capacity * 2
	}

	return &WorkerPool{
		capacity:  capacity,
		taskQueue: make(chan Task, queueSize),
		stopChan:  make(chan struct{}),
	}, nil
}

func (p *WorkerPool) Start(ctx context.Context, workerFunc WorkerFunc) error {
	if p.stopped.Load() {
		return ErrPoolStopped
	}

	for i := 0; i < p.capacity; i++ {
		p.wg.Add(1)
		go p.worker(ctx, workerFunc, i)
	}

	return nil
}

func (p *WorkerPool) Submit(ctx context.Context, taskID int, file fs.FileInfo, resultChan chan<- TaskResult) error {
	if p.stopped.Load() {
		return ErrPoolStopped
	}

	select {
	case p.taskQueue <- Task{
		ID:     taskID,
		File:   file,
		Result: resultChan,
		Ctx:    ctx,
	}:
		p.totalTasks.Add(1)
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return ErrPoolBusy
	}
}

// SubmitBatch submits multiple tasks to the worker pool
func (p *WorkerPool) SubmitBatch(ctx context.Context, files []fs.FileInfo, resultChan chan<- TaskResult) error {
	for i, file := range files {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			if err := p.Submit(ctx, i, file, resultChan); err != nil {
				return fmt.Errorf("failed to submit task %d: %w", i, err)
			}
		}
	}
	return nil
}

// ActiveWorkers returns the number of currently active workers
func (p *WorkerPool) ActiveWorkers() int {
	return int(p.activeWorkers.Load())
}

// QueueSize returns the current queue size
func (p *WorkerPool) QueueSize() int {
	return len(p.taskQueue)
}

// Stats returns pool statistics
func (p *WorkerPool) Stats() map[string]int64 {
	return map[string]int64{
		"capacity":      int64(p.capacity),
		"queue_size":    int64(len(p.taskQueue)),
		"active":        int64(p.ActiveWorkers()),
		"total_tasks":   p.totalTasks.Load(),
		"failed_tasks":  p.failedTasks.Load(),
		"retried_tasks": p.retriedTasks.Load(),
	}
}

// Stop stops the worker pool gracefully
func (p *WorkerPool) Stop() error {
	if p.stopped.Swap(true) {
		return nil // Already stopped
	}

	close(p.stopChan)

	// Wait for all workers to finish
	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()

	// Wait with timeout
	select {
	case <-done:
		return nil
	case <-time.After(5 * time.Second):
		return errors.New("timeout waiting for worker pool to stop")
	}
}

// worker is the goroutine that processes tasks
func (p *WorkerPool) worker(ctx context.Context, workerFunc WorkerFunc, id int) {
	defer p.wg.Done()

	for {
		select {
		case <-p.stopChan:
			return
		case task := <-p.taskQueue:
			p.processTask(ctx, task, workerFunc, id)
		case <-ctx.Done():
			return
		}
	}
}

// processTask processes a single task
func (p *WorkerPool) processTask(ctx context.Context, task Task, workerFunc WorkerFunc, workerID int) {
	p.activeWorkers.Add(1)
	defer p.activeWorkers.Add(-1)

	// Merge contexts
	mergedCtx, cancel := context.WithTimeout(task.Ctx, 30*time.Second)
	defer cancel()

	// Process the task
	issues, err := workerFunc(mergedCtx, task.File)

	select {
	case <-mergedCtx.Done():
		// Context was cancelled or timed out
		if errors.Is(mergedCtx.Err(), context.DeadlineExceeded) {
			p.failedTasks.Add(1)
			task.Result <- TaskResult{
				TaskID: task.ID,
				File:   task.File,
				Error:  fmt.Errorf("review timed out after 30 seconds"),
				Retry:  true,
			}
		} else {
			p.failedTasks.Add(1)
			task.Result <- TaskResult{
				TaskID: task.ID,
				File:   task.File,
				Error:  mergedCtx.Err(),
				Retry:  false,
			}
		}
	default:
		// Send result
		task.Result <- TaskResult{
			TaskID: task.ID,
			File:   task.File,
			Issues: issues,
			Error:  err,
			Retry:  err != nil, // Retry on error
		}

		if err != nil {
			p.failedTasks.Add(1)
		}
	}
}

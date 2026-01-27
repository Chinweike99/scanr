package worker

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"scanr/internal/fs"
)

func TestNewWorkerPool(t *testing.T) {
	tests := []struct {
		name      string
		capacity  int
		queueSize int
		wantErr   bool
	}{
		{"valid capacity", 4, 10, false},
		{"zero capacity", 0, 10, true},
		{"negative capacity", -1, 10, true},
		{"negative queue size", 4, -1, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewWorkerPool(tt.capacity, tt.queueSize)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestWorkerPool_Submit(t *testing.T) {
	pool, err := NewWorkerPool(2, 5)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Stop()

	ctx := context.Background()

	// Start the pool
	var processedTasks int32
	workerFunc := func(ctx context.Context, file *fs.FileInfo) (interface{}, error) {
		atomic.AddInt32(&processedTasks, 1)
		time.Sleep(10 * time.Millisecond)
		return nil, nil
	}

	if err := pool.Start(ctx, workerFunc); err != nil {
		t.Fatal(err)
	}

	// Submit tasks
	resultChan := make(chan TaskResult, 5)
	file := &fs.FileInfo{Path: "/test/file.go"}

	for i := 0; i < 5; i++ {
		if err := pool.Submit(ctx, i, file, resultChan); err != nil {
			t.Fatalf("Submit failed: %v", err)
		}
	}

	// Wait for results
	timeout := time.After(1 * time.Second)
	for i := 0; i < 5; i++ {
		select {
		case <-resultChan:
			// Got result
		case <-timeout:
			t.Fatal("timeout waiting for results")
		}
	}

	if processedTasks != 5 {
		t.Errorf("expected 5 processed tasks, got %d", processedTasks)
	}
}

func TestWorkerPool_Backpressure(t *testing.T) {
	pool, err := NewWorkerPool(1, 2) // Small capacity and queue
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Stop()

	ctx := context.Background()

	// Create a worker that blocks on a channel to keep workers busy
	blockChan := make(chan struct{})
	workerFunc := func(ctx context.Context, file *fs.FileInfo) (interface{}, error) {
		<-blockChan // Block until we unblock it
		return nil, nil
	}

	if err := pool.Start(ctx, workerFunc); err != nil {
		t.Fatal(err)
	}

	// Give worker time to start waiting on the queue
	time.Sleep(10 * time.Millisecond)

	// Fill the queue quickly
	resultChan := make(chan TaskResult, 5)
	file := &fs.FileInfo{Path: "/test/file.go"}

	submitted := 0
	for i := 0; i < 5; i++ {
		err := pool.Submit(ctx, i, file, resultChan)
		if err == nil {
			submitted++
		} else if err == ErrPoolBusy {
			break // Queue is full as expected
		} else {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	// Should have submitted capacity + queueSize tasks before getting busy
	if submitted <= 2 { // 1 worker + 2 queue = 3 slots
		t.Errorf("expected at least 3 submissions before backpressure, got %d", submitted)
	}

	// Unblock workers
	close(blockChan)
}

func TestWorkerPool_ContextCancellation(t *testing.T) {
	pool, err := NewWorkerPool(2, 5)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Stop()

	ctx, cancel := context.WithCancel(context.Background())

	workerFunc := func(ctx context.Context, file *fs.FileInfo) (interface{}, error) {
		select {
		case <-time.After(100 * time.Millisecond):
			return nil, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	if err := pool.Start(ctx, workerFunc); err != nil {
		t.Fatal(err)
	}

	// Give worker time to start
	time.Sleep(10 * time.Millisecond)

	// Submit a task
	resultChan := make(chan TaskResult, 1)
	file := &fs.FileInfo{Path: "/test/file.go"}

	if err := pool.Submit(ctx, 1, file, resultChan); err != nil {
		t.Fatal(err)
	}

	// Cancel context before task completes
	cancel()

	// Check result
	select {
	case result := <-resultChan:
		if result.Error == nil {
			t.Error("expected error due to context cancellation")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for result")
	}
}

func TestWorkerPool_Stop(t *testing.T) {
	pool, err := NewWorkerPool(2, 5)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start pool with workers that complete quickly
	workerFunc := func(ctx context.Context, file *fs.FileInfo) (interface{}, error) {
		// Just return immediately
		return nil, nil
	}

	if err := pool.Start(ctx, workerFunc); err != nil {
		t.Fatal(err)
	}

	// Submit a task to ensure workers are running
	resultChan := make(chan TaskResult, 1)
	file := &fs.FileInfo{Path: "/test/file.go"}
	pool.Submit(ctx, 1, file, resultChan)

	// Give workers time to pick up task
	time.Sleep(50 * time.Millisecond)

	// Stop the pool
	stopDone := make(chan struct{})
	go func() {
		if err := pool.Stop(); err != nil {
			t.Errorf("Stop failed: %v", err)
		}
		close(stopDone)
	}()

	// Wait for stop with timeout
	select {
	case <-stopDone:
		// Success
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for pool to stop")
	}

	// Verify cannot submit after stop
	err = pool.Submit(ctx, 2, file, resultChan)
	if err != ErrPoolStopped {
		t.Errorf("expected ErrPoolStopped after stop, got %v", err)
	}
}

func TestWorkerPool_Stats(t *testing.T) {
	pool, err := NewWorkerPool(3, 10)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Stop()

	ctx := context.Background()

	workerFunc := func(ctx context.Context, file *fs.FileInfo) (interface{}, error) {
		time.Sleep(50 * time.Millisecond)
		return nil, nil
	}

	if err := pool.Start(ctx, workerFunc); err != nil {
		t.Fatal(err)
	}

	// Submit some tasks
	resultChan := make(chan TaskResult, 5)
	file := &fs.FileInfo{Path: "/test/file.go"}

	for i := 0; i < 5; i++ {
		pool.Submit(ctx, i, file, resultChan)
	}

	// Wait a bit for tasks to start processing
	time.Sleep(10 * time.Millisecond)

	stats := pool.Stats()

	if stats["capacity"] != 3 {
		t.Errorf("expected capacity 3, got %d", stats["capacity"])
	}

	if stats["total_tasks"] != 5 {
		t.Errorf("expected 5 total tasks, got %d", stats["total_tasks"])
	}

	// Should have some active workers
	if stats["active"] == 0 {
		t.Error("expected some active workers")
	}
}

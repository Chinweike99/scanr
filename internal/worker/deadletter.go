package worker

import (
    "fmt"
    "sync"
    "time"
)

// DeadLetter represents a task that failed processing
type DeadLetter struct {
    Task      Task
    Error     error
    Timestamp time.Time
    Attempts  int
}

// DeadLetterQueue manages failed tasks that can be retried or logged
type DeadLetterQueue struct {
    items     []DeadLetter
    mu        sync.RWMutex
    maxSize   int
    onDiscard func(dl DeadLetter)
}

//creates a new dead letter queue
func NewDeadLetterQueue(maxSize int) *DeadLetterQueue {
    return &DeadLetterQueue{
        items:   make([]DeadLetter, 0, maxSize),
        maxSize: maxSize,
        onDiscard: func(dl DeadLetter) {
            fmt.Printf("Discarded dead letter after %d attempts: %v\n", 
                dl.Attempts, dl.Error)
        },
    }
}

// adds a failed task to the dead letter queue
func (q *DeadLetterQueue) Push(task Task, err error, attempts int) {
    q.mu.Lock()
    defer q.mu.Unlock()
    
    dl := DeadLetter{
        Task:      task,
        Error:     err,
        Timestamp: time.Now(),
        Attempts:  attempts,
    }
    
    if len(q.items) >= q.maxSize {
        if q.onDiscard != nil {
            q.onDiscard(q.items[0])
        }
        q.items = q.items[1:]
    }
    
    q.items = append(q.items, dl)
}


func (q *DeadLetterQueue) Pop() (DeadLetter, bool) {
    q.mu.Lock()
    defer q.mu.Unlock()
    
    if len(q.items) == 0 {
        return DeadLetter{}, false
    }
    
    dl := q.items[0]
    q.items = q.items[1:]
    return dl, true
}

// Peek returns the oldest dead letter without removing it
func (q *DeadLetterQueue) Peek() (DeadLetter, bool) {
    q.mu.RLock()
    defer q.mu.RUnlock()
    
    if len(q.items) == 0 {
        return DeadLetter{}, false
    }
    
    return q.items[0], true
}

// Size returns the current number of items in the queue
func (q *DeadLetterQueue) Size() int {
    q.mu.RLock()
    defer q.mu.RUnlock()
    return len(q.items)
}

// Clear removes all items from the queue
func (q *DeadLetterQueue) Clear() {
    q.mu.Lock()
    defer q.mu.Unlock()
    q.items = q.items[:0]
}

// Items returns a copy of all items in the queue
func (q *DeadLetterQueue) Items() []DeadLetter {
    q.mu.RLock()
    defer q.mu.RUnlock()
    
    items := make([]DeadLetter, len(q.items))
    copy(items, q.items)
    return items
}

// SetDiscardHandler sets a custom handler for discarded items
func (q *DeadLetterQueue) SetDiscardHandler(handler func(DeadLetter)) {
    q.mu.Lock()
    defer q.mu.Unlock()
    q.onDiscard = handler
}
package taskqueue

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gammazero/workerpool"
)

// Task expiration time
var TaskExpirationTime = 4 * time.Hour

// ErrTaskExpired is returned when a task has expired
var ErrTaskExpired = errors.New("task expired")

// AnyTask is an interface for tasks that can be executed
type AnyTask interface {
	Execute() error
	ShouldRetry(error) bool
	IsExpired() bool
}

// Task is a generic implementation of AnyTask
type Task[T any] struct {
	ExecuteFunc func() (T, error)
	Callback    func(T, error)
	RetryError  error
	CreatedAt   time.Time
}

// NewTask creates a new task with the given execute function, callback, and retry error
func NewTask[T any](
	executeFunc func() (T, error),
	callback func(T, error),
	retryError error,
) Task[T] {
	return Task[T]{
		ExecuteFunc: executeFunc,
		Callback:    callback,
		RetryError:  retryError,
		CreatedAt:   time.Now(),
	}
}

// Execute executes the task and calls the callback with the result
func (t Task[T]) Execute() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic in task execution: %v", r)
		}
	}()

	// Check if the task has expired
	if t.IsExpired() {
		return ErrTaskExpired
	}

	result, err := t.ExecuteFunc()
	t.Callback(result, err)
	return err
}

// ShouldRetry returns true if the error is the retry error
func (t Task[T]) ShouldRetry(err error) bool {
	return errors.Is(err, t.RetryError)
}

// IsExpired returns true if the task was created more than TaskExpirationTime ago
func (t Task[T]) IsExpired() bool {
	return time.Since(t.CreatedAt) > TaskExpirationTime
}

// Queue is a task queue that executes tasks sequentially
type Queue struct {
	pool *workerpool.WorkerPool
	wg   sync.WaitGroup
}

// NewQueue creates a new task queue
func NewQueue() *Queue {
	// Use a pool size of 1 to ensure sequential execution
	q := &Queue{
		pool: workerpool.New(1),
	}
	return q
}

// Add adds a task to the queue
func (q *Queue) Add(task AnyTask) {
	q.wg.Add(1)
	q.pool.Submit(func() {
		q.processTask(task)
	})
}

// processTask processes a task and retries it if necessary
func (q *Queue) processTask(task AnyTask) {
	defer q.wg.Done()

	// Skip expired tasks
	if task.IsExpired() {
		return
	}

	err := task.Execute()
	if err != nil {
		if errors.Is(err, ErrTaskExpired) {
			// Task expired, don't retry
			return
		}
		if task.ShouldRetry(err) {
			// Resubmit the task
			q.Add(task)
		}
	}
}

// Wait waits for all tasks to complete
func (q *Queue) Wait() {
	q.wg.Wait()
}

// Close stops the worker pool and waits for all tasks to complete
func (q *Queue) Close() {
	q.pool.Stop()
	q.wg.Wait()
}

package tq

import (
	"errors"
	"fmt"
	"sync"
)

type AnyTask interface {
	Execute() error
	ShouldRetry(error) bool
}

type Task[T any] struct {
	ExecuteFunc func() (T, error)
	Callback    func(T, error)
	RetryError  error
}

func NewTask[T any](
	executeFunc func() (T, error),
	callback func(T, error),
	retryError error,
) Task[T] {
	return Task[T]{
		ExecuteFunc: executeFunc,
		Callback:    callback,
		RetryError:  retryError,
	}
}

func (t Task[T]) Execute() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic in task execution: %v", r)
		}
	}()

	result, err := t.ExecuteFunc()
	t.Callback(result, err)
	return err
}

func (t Task[T]) ShouldRetry(err error) bool {
	return errors.Is(err, t.RetryError)
}

type Queue struct {
	tasks chan AnyTask
	wg    sync.WaitGroup
}

func NewQueue(size int) *Queue {
	q := &Queue{
		tasks: make(chan AnyTask, size),
	}
	q.startWorker()
	return q
}

func (q *Queue) Add(task AnyTask) {
	q.wg.Add(1)
	q.tasks <- task
}

func (q *Queue) startWorker() {
	go func() {
		for task := range q.tasks {
			q.processTask(task)
		}
	}()
}

func (q *Queue) processTask(task AnyTask) {
	defer q.wg.Done()
	err := task.Execute()
	if err != nil && task.ShouldRetry(err) {
		q.Add(task)
	}
}

func (q *Queue) Wait() {
	q.wg.Wait()
}

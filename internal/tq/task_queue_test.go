package tq

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestHeterogeneousQueue(t *testing.T) {
	queue := NewQueue(10)
	var mu sync.Mutex
	var results []string

	stringTask := NewTask(
		func() (string, error) {
			mu.Lock()
			defer mu.Unlock()
			results = append(results, "executed string task")
			return "success", nil
		},
		func(result string, err error) {
			mu.Lock()
			defer mu.Unlock()
			results = append(results, fmt.Sprintf("string callback with result: %v", result))
		},
		nil,
	)

	intTask := NewTask(
		func() (int, error) {
			mu.Lock()
			defer mu.Unlock()
			results = append(results, "executed int task")
			return 42, nil
		},
		func(result int, err error) {
			mu.Lock()
			defer mu.Unlock()
			results = append(results, fmt.Sprintf("int callback with result: %v", result))
		},
		nil,
	)

	retryError := errors.New("retry error")
	retryCount := 0
	retryTask := NewTask(
		func() (bool, error) {
			mu.Lock()
			defer mu.Unlock()
			results = append(results, "executed retry task")
			if retryCount < 2 {
				retryCount++
				return false, retryError
			}
			return true, nil
		},
		func(result bool, err error) {
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				results = append(results, "retry task error")
			} else {
				results = append(results, fmt.Sprintf("retry task success: %v", result))
			}
		},
		retryError,
	)

	queue.Add(stringTask)
	queue.Add(intTask)
	queue.Add(retryTask)

	// Wait for all tasks to complete
	done := make(chan bool)
	go func() {
		queue.Wait()
		done <- true
	}()

	select {
	case <-done:
		// All tasks completed
	case <-time.After(5 * time.Second):
		t.Fatal("Test timed out")
	}

	expectedResults := []string{
		"executed string task",
		"string callback with result: success",
		"executed int task",
		"int callback with result: 42",
		"executed retry task",
		"retry task error",
		"executed retry task",
		"retry task error",
		"executed retry task",
		"retry task success: true",
	}

	if len(results) != len(expectedResults) {
		t.Errorf("Expected %d results, got %d", len(expectedResults), len(results))
		t.Errorf("Actual results: %v", results)
		return
	}

	for i, result := range results {
		if result != expectedResults[i] {
			t.Errorf("Expected result %d to be %s, got %s", i, expectedResults[i], result)
		}
	}
}

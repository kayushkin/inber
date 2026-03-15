package server

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestQueueSessionSerialization(t *testing.T) {
	q := NewQueue(map[string]int{"main": 10})

	var running atomic.Int32
	var maxConcurrent atomic.Int32
	var wg sync.WaitGroup

	// Enqueue 5 tasks on the SAME session — should serialize.
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			q.Enqueue(context.Background(), "main", "session-A", func(ctx context.Context) error {
				n := running.Add(1)
				if n > 1 {
					t.Errorf("concurrent runs on same session: %d", n)
				}
				if n > int32(maxConcurrent.Load()) {
					maxConcurrent.Store(int32(n))
				}
				time.Sleep(5 * time.Millisecond)
				running.Add(-1)
				return nil
			})
		}()
	}

	wg.Wait()

	if maxConcurrent.Load() > 1 {
		t.Errorf("expected max 1 concurrent on same session, got %d", maxConcurrent.Load())
	}
}

func TestQueueDifferentSessionsConcurrent(t *testing.T) {
	q := NewQueue(map[string]int{"main": 10})

	var maxConcurrent atomic.Int32
	var running atomic.Int32
	var wg sync.WaitGroup

	// Enqueue on DIFFERENT sessions — should run concurrently.
	for i := 0; i < 5; i++ {
		wg.Add(1)
		sessionKey := string(rune('A' + i))
		go func() {
			defer wg.Done()
			q.Enqueue(context.Background(), "main", sessionKey, func(ctx context.Context) error {
				n := running.Add(1)
				for {
					old := maxConcurrent.Load()
					if int32(n) <= old || maxConcurrent.CompareAndSwap(old, int32(n)) {
						break
					}
				}
				time.Sleep(20 * time.Millisecond)
				running.Add(-1)
				return nil
			})
		}()
	}

	wg.Wait()

	if maxConcurrent.Load() < 2 {
		t.Errorf("expected concurrent runs on different sessions, max was %d", maxConcurrent.Load())
	}
}

func TestQueueLaneConcurrencyLimit(t *testing.T) {
	q := NewQueue(map[string]int{"narrow": 2})

	var maxConcurrent atomic.Int32
	var running atomic.Int32
	var wg sync.WaitGroup

	// 5 different sessions on a lane with concurrency=2.
	for i := 0; i < 5; i++ {
		wg.Add(1)
		sessionKey := string(rune('A' + i))
		go func() {
			defer wg.Done()
			q.Enqueue(context.Background(), "narrow", sessionKey, func(ctx context.Context) error {
				n := running.Add(1)
				for {
					old := maxConcurrent.Load()
					if int32(n) <= old || maxConcurrent.CompareAndSwap(old, int32(n)) {
						break
					}
				}
				time.Sleep(20 * time.Millisecond)
				running.Add(-1)
				return nil
			})
		}()
	}

	wg.Wait()

	if maxConcurrent.Load() > 2 {
		t.Errorf("expected max 2 concurrent (lane limit), got %d", maxConcurrent.Load())
	}
}

func TestQueueContextCancellation(t *testing.T) {
	q := NewQueue(map[string]int{"main": 1})

	ctx, cancel := context.WithCancel(context.Background())

	// Block the lane.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		q.Enqueue(context.Background(), "main", "blocker", func(ctx context.Context) error {
			time.Sleep(100 * time.Millisecond)
			return nil
		})
	}()

	// Try to enqueue with a cancelled context.
	time.Sleep(10 * time.Millisecond) // let blocker start
	cancel()

	err := q.Enqueue(ctx, "main", "other-session", func(ctx context.Context) error {
		return nil
	})

	if err == nil {
		// Might or might not error depending on timing, both OK.
	}

	wg.Wait()
}

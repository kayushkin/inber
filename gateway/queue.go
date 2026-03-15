package gateway

import (
	"context"
	"sync"
)

// Queue manages lane-based concurrent work with per-session serialization.
// Same session key = serialized (never two runs on one session).
// Different sessions in same lane = parallel up to lane concurrency.
// Different lanes = fully independent.
type Queue struct {
	lanes    map[string]*lane
	sessions sync.Map // sessionKey → *sync.Mutex
	mu       sync.Mutex
}

type lane struct {
	sem chan struct{} // buffered to concurrency limit
}

// NewQueue creates a queue with the given lane concurrency limits.
func NewQueue(lanes map[string]int) *Queue {
	q := &Queue{
		lanes: make(map[string]*lane),
	}
	for name, concurrency := range lanes {
		if concurrency < 1 {
			concurrency = 1
		}
		q.lanes[name] = &lane{
			sem: make(chan struct{}, concurrency),
		}
	}
	return q
}

// Enqueue runs work in the specified lane, serialized by session key.
// Blocks until a lane slot opens AND no other work is running on this session.
func (q *Queue) Enqueue(ctx context.Context, laneName, sessionKey string, work func(ctx context.Context) error) error {
	// 1. Acquire per-session lock (serialization).
	smu := q.getSessionMutex(sessionKey)
	smu.Lock()
	defer smu.Unlock()

	// 2. Acquire lane slot (concurrency cap).
	l := q.getLane(laneName)
	select {
	case l.sem <- struct{}{}:
		defer func() { <-l.sem }()
	case <-ctx.Done():
		return ctx.Err()
	}

	// 3. Run.
	return work(ctx)
}

func (q *Queue) getSessionMutex(key string) *sync.Mutex {
	v, _ := q.sessions.LoadOrStore(key, &sync.Mutex{})
	return v.(*sync.Mutex)
}

func (q *Queue) getLane(name string) *lane {
	q.mu.Lock()
	defer q.mu.Unlock()
	l, ok := q.lanes[name]
	if !ok {
		// Default lane with concurrency 1.
		l = &lane{sem: make(chan struct{}, 1)}
		q.lanes[name] = l
	}
	return l
}

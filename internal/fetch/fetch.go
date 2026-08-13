// Package fetch is how one invocation asks Proton for things: each thing at most
// once, and requests that do not need each other's answers at the same time.
//
// Both halves exist because straight-line Go makes a request graph look like a
// chain. A command that reads one event needs eight things from the server and
// none of them depends on another, so writing them one after the other costs
// eight round trips to learn what one round trip could have said. Wall time
// should be the depth of the graph rather than its size.
//
// Neither half caches anything beyond the life of its holder, which is one
// invocation. A response reused across runs is a response nobody parsed, and a
// change to it would then pass unnoticed.
package fetch

import (
	"context"
	"sync"
)

// Together makes every request at the same time and returns the first error in
// argument order, so which failure is reported does not depend on which lost a
// race.
//
// Every request is waited for. Cancelling the rest on the first failure would
// save a little time and cost the caller a truthful error from every one of them.
func Together(ctx context.Context, requests ...func(context.Context) error) error {
	failures := make([]error, len(requests))
	var wg sync.WaitGroup
	for i, request := range requests {
		wg.Add(1)
		go func() {
			defer wg.Done()
			failures[i] = request(ctx)
		}()
	}
	wg.Wait()
	for _, err := range failures {
		if err != nil {
			return err
		}
	}
	return nil
}

// Memo holds what has already been fetched, keyed by whatever names it - a
// calendar ID, a share ID, or the empty string for the one thing of its kind.
//
// A failure is remembered like an answer. The client has already retried what is
// worth retrying by the time an error reaches here, so asking again would only
// fail again, more slowly.
type Memo[T any] struct {
	mu      sync.Mutex
	entries map[string]*answer[T]
}

type answer[T any] struct {
	once  sync.Once
	value T
	err   error
}

// Do returns what load produced the first time it was asked for this key.
// Concurrent callers for one key wait for the first, so a fan-out that wants the
// same thing twice still fetches it once.
func (m *Memo[T]) Do(key string, load func() (T, error)) (T, error) {
	m.mu.Lock()
	if m.entries == nil {
		m.entries = make(map[string]*answer[T])
	}
	a, ok := m.entries[key]
	if !ok {
		a = &answer[T]{}
		m.entries[key] = a
	}
	m.mu.Unlock()

	a.once.Do(func() { a.value, a.err = load() })
	return a.value, a.err
}
